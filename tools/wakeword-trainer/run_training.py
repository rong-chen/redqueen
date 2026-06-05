import os
import sys
import platform
import subprocess
import yaml

# ---------------------------------------------------------
# 1. 配置区域
# ---------------------------------------------------------
# 您想训练的唤醒词列表（纯英文合成使用拼音或英文）
# 您可以放多个！模型会同时学习这两个词汇，听到任何一个都会唤醒。
target_words = ['red_queen', 'queen']  

# 如果您自己录制了真实的录音（推荐），请将所有 wav 文件放入 'custom_samples' 文件夹，
# 并将 use_custom_samples 设置为 True
use_custom_samples = False

print(f"=== 开始配置环境并准备合成唤醒词: {', '.join(target_words)} ===")

# ---------------------------------------------------------
# 2. 生成合成样本 (如果未使用自有录音)
# ---------------------------------------------------------
if not use_custom_samples:
    print(">>> 正在使用 Piper 合成唤醒词音频...")
    if not os.path.exists("./piper-sample-generator"):
        print("错误: 找不到 piper-sample-generator！请确保脚本运行在正确的虚拟环境中。")
        sys.exit(1)
        
    if not os.path.exists("piper-sample-generator/models/en_US-libritts_r-medium.pt"):
        print(">>> 下载 TTS 模型...")
        os.makedirs("piper-sample-generator/models", exist_ok=True)
        os.system("curl -L -o piper-sample-generator/models/en_US-libritts_r-medium.pt 'https://github.com/rhasspy/piper-sample-generator/releases/download/v2.0.0/en_US-libritts_r-medium.pt'")
    
    # 将项目路径加入 PYTHONPATH
    if "piper-sample-generator/" not in sys.path:
        sys.path.append("piper-sample-generator/")
        
    print(f">>> 开始批量合成正向样本 (共 {len(target_words)} 个词汇，可能会花一些时间)...")
    for word in target_words:
        print(f"  -> 正在合成: {word}")
        # 这里每个词合成 1000 个样本放入同一个 generated_samples 文件夹
        os.system(f'python3 piper-sample-generator/generate_samples.py "{word}" --max-samples 1000 --batch-size 100 --output-dir generated_samples')
    
    input_directory = 'generated_samples'
else:
    print(">>> 使用您提供的真实录音...")
    input_directory = 'custom_samples'
    if not os.path.exists(input_directory):
        print(f"错误: 找不到 {input_directory} 文件夹！请创建并放入您的录音。")
        sys.exit(1)


# ---------------------------------------------------------
# 3. 下载噪音与环境声数据
# ---------------------------------------------------------
import datasets
import scipy.io.wavfile
import numpy as np
from pathlib import Path
from tqdm import tqdm

print("=== 开始下载和准备环境噪音数据 (只需要执行一次) ===")

## 下载空间脉冲响应 (RIR)
output_dir = "./mit_rirs"
if not os.path.exists(output_dir):
    print(">>> 下载 MIT RIR (空间环境声学回声) ...")
    os.mkdir(output_dir)
    rir_dataset = datasets.load_dataset("davidscripka/MIT_environmental_impulse_responses", split="train", streaming=True)
    for row in tqdm(rir_dataset, desc="Saving RIRs"):
        name = row['audio']['path'].split('/')[-1]
        scipy.io.wavfile.write(os.path.join(output_dir, name), 16000, (row['audio']['array']*32767).astype(np.int16))

## 下载 AudioSet 噪音
if not os.path.exists("audioset_16k"):
    print(">>> 下载 AudioSet 噪音库 (可能较慢) ...")
    os.mkdir("audioset")
    fname = "bal_train09.tar"
    out_dir = f"audioset/{fname}"
    link = "https://huggingface.co/datasets/agkphysics/AudioSet/resolve/main/data/" + fname
    os.system(f"curl -L -o {out_dir} {link}")
    os.system("cd audioset && tar -xf bal_train09.tar")

    os.mkdir("audioset_16k")
    audioset_dataset = datasets.Dataset.from_dict({"audio": [str(i) for i in Path("audioset/audio").glob("**/*.flac")]})
    audioset_dataset = audioset_dataset.cast_column("audio", datasets.Audio(sampling_rate=16000))
    for row in tqdm(audioset_dataset, desc="Converting AudioSet to 16kHz"):
        name = row['audio']['path'].split('/')[-1].replace(".flac", ".wav")
        scipy.io.wavfile.write(os.path.join("audioset_16k", name), 16000, (row['audio']['array']*32767).astype(np.int16))

## 下载 FMA 音乐背景库
if not os.path.exists("fma_16k"):
    print(">>> 下载 FMA 音乐噪音库 ...")
    os.mkdir("fma")
    fname = "fma_xs.zip"
    link = "https://huggingface.co/datasets/mchl914/fma_xsmall/resolve/main/" + fname
    out_dir = f"fma/{fname}"
    os.system(f"curl -L -o {out_dir} {link}")
    os.system(f"cd fma && unzip -q {fname}")

    os.mkdir("fma_16k")
    fma_dataset = datasets.Dataset.from_dict({"audio": [str(i) for i in Path("fma/fma_small").glob("**/*.mp3")]})
    fma_dataset = fma_dataset.cast_column("audio", datasets.Audio(sampling_rate=16000))
    for row in tqdm(fma_dataset, desc="Converting FMA to 16kHz"):
        name = row['audio']['path'].split('/')[-1].replace(".mp3", ".wav")
        scipy.io.wavfile.write(os.path.join("fma_16k", name), 16000, (row['audio']['array']*32767).astype(np.int16))


# ---------------------------------------------------------
# 4. 音频增强与频谱提取
# ---------------------------------------------------------
print("=== 设置声音暴力增强 (混音/回声/加噪) ===")
from microwakeword.audio.augmentation import Augmentation
from microwakeword.audio.clips import Clips
from microwakeword.audio.spectrograms import SpectrogramGeneration
from mmap_ninja.ragged import RaggedMmap

clips = Clips(input_directory=input_directory,
              file_pattern='*.wav',
              max_clip_duration_s=None,
              remove_silence=False,
              random_split_seed=10,
              split_count=0.1,
              )
augmenter = Augmentation(augmentation_duration_s=3.2,
                         augmentation_probabilities = {
                                "SevenBandParametricEQ": 0.1,
                                "TanhDistortion": 0.1,
                                "PitchShift": 0.1,
                                "BandStopFilter": 0.1,
                                "AddColorNoise": 0.1,
                                "AddBackgroundNoise": 0.75,
                                "Gain": 1.0,
                                "RIR": 0.5,
                            },
                         impulse_paths = ['mit_rirs'],
                         background_paths = ['fma_16k', 'audioset_16k'],
                         background_min_snr_db = -5,
                         background_max_snr_db = 10,
                         min_jitter_s = 0.195,
                         max_jitter_s = 0.205,
                         )

output_dir = 'generated_augmented_features'
if not os.path.exists(output_dir):
    os.mkdir(output_dir)

print(">>> 开始生成验证/训练用的频谱特征图...")
splits = ["training", "validation", "testing"]
for split in splits:
  out_dir = os.path.join(output_dir, split)
  if not os.path.exists(out_dir):
      os.mkdir(out_dir)

  split_name = "train"
  repetition = 2

  spectrograms = SpectrogramGeneration(clips=clips, augmenter=augmenter, slide_frames=10, step_ms=10)
  if split == "validation":
    split_name = "validation"
    repetition = 1
  elif split == "testing":
    split_name = "test"
    repetition = 1
    spectrograms = SpectrogramGeneration(clips=clips, augmenter=augmenter, slide_frames=1, step_ms=10)

  mmap_path = os.path.join(out_dir, 'wakeword_mmap')
  if not os.path.exists(mmap_path):
      print(f"生成 {split} 特征...")
      RaggedMmap.from_generator(
          out_dir=mmap_path,
          sample_generator=spectrograms.spectrogram_generator(split=split_name, repeat=repetition),
          batch_size=100,
          verbose=True,
      )


# ---------------------------------------------------------
# 5. 下载预置负样本数据
# ---------------------------------------------------------
neg_output_dir = './negative_datasets'
if not os.path.exists(neg_output_dir):
    print("=== 下载官方负样本对抗数据 ===")
    os.mkdir(neg_output_dir)
    link_root = "https://huggingface.co/datasets/kahrendt/microwakeword/resolve/main/"
    filenames = ['dinner_party.zip', 'dinner_party_eval.zip', 'no_speech.zip', 'speech.zip']
    for fname in filenames:
        link = link_root + fname
        zip_path = f"negative_datasets/{fname}"
        os.system(f"curl -L -o {zip_path} {link}")
        os.system(f"unzip -q {zip_path} -d {neg_output_dir}")


# ---------------------------------------------------------
# 6. 配置并开始模型训练！
# ---------------------------------------------------------
print("=== 准备开始神经网络训练 ===")
config = {
    "window_step_ms": 10,
    "train_dir": "trained_models/wakeword",
    "features": [
        {
            "features_dir": "generated_augmented_features",
            "sampling_weight": 2.0,
            "penalty_weight": 1.0,
            "truth": True,
            "truncation_strategy": "truncate_start",
            "type": "mmap",
        },
        {
            "features_dir": "negative_datasets/speech",
            "sampling_weight": 10.0,
            "penalty_weight": 1.0,
            "truth": False,
            "truncation_strategy": "random",
            "type": "mmap",
        },
        {
            "features_dir": "negative_datasets/dinner_party",
            "sampling_weight": 10.0,
            "penalty_weight": 1.0,
            "truth": False,
            "truncation_strategy": "random",
            "type": "mmap",
        },
        {
            "features_dir": "negative_datasets/no_speech",
            "sampling_weight": 5.0,
            "penalty_weight": 1.0,
            "truth": False,
            "truncation_strategy": "random",
            "type": "mmap",
        },
        { 
            "features_dir": "negative_datasets/dinner_party_eval",
            "sampling_weight": 0.0,
            "penalty_weight": 1.0,
            "truth": False,
            "truncation_strategy": "split",
            "type": "mmap",
        },
    ],
    "training_steps": [10000], # 这里可以改成20000或更多，训练得更久
    "positive_class_weight": [1],
    "negative_class_weight": [20],
    "learning_rates": [0.001],
    "batch_size": 128,
    "time_mask_max_size": [0],
    "time_mask_count": [0],
    "freq_mask_max_size": [0],
    "freq_mask_count": [0],
    "eval_step_interval": 500,
    "clip_duration_ms": 1500,
    "target_minimization": 0.9,
    "minimization_metric": None,
    "maximization_metric": "average_viable_recall",
}

with open("training_parameters.yaml", "w") as file:
    yaml.dump(config, file)

print(">>> 启动 microWakeWord 训练进程，这会占用一段时间的 CPU/GPU，并最终输出 .tflite 模型！")
os.system(
    "python3 -m microwakeword.model_train_eval "
    "--training_config='training_parameters.yaml' "
    "--train 1 "
    "--restore_checkpoint 1 "
    "--test_tf_nonstreaming 0 "
    "--test_tflite_nonstreaming 0 "
    "--test_tflite_nonstreaming_quantized 0 "
    "--test_tflite_streaming 0 "
    "--test_tflite_streaming_quantized 1 "
    "--use_weights 'best_weights' "
    "mixednet "
    "--pointwise_filters '64,64,64,64' "
    "--repeat_in_block '1, 1, 1, 1' "
    "--mixconv_kernel_sizes '[5], [7,11], [9,15], [23]' "
    "--residual_connection '0,0,0,0' "
    "--first_conv_filters 32 "
    "--first_conv_kernel_size 5 "
    "--stride 3"
)

print("=== 训练结束！===")
print("请去以下目录寻找您的 .tflite 模型文件：")
print("wake-word-trainer/trained_models/wakeword/tflite_stream_state_internal_quant/stream_state_internal_quant.tflite")
