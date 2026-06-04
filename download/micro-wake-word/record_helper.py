#!/usr/bin/env python3
import os
import sys
import wave
import time

# Auto install dependency helper
try:
    import sounddevice as sd
    import numpy as np
except ImportError:
    print("=== [Helper] Installing required libraries (sounddevice, numpy) ===")
    import subprocess
    subprocess.check_call([sys.executable, "-m", "pip", "install", "sounddevice", "numpy"])
    import sounddevice as sd
    import numpy as np

# Audio config for microWakeWord
SAMPLE_RATE = 16000
CHANNELS = 1
OUTPUT_DIR = "my_samples"

def record_sample(file_path):
    print("\n准备就绪！")
    input("--> 按 [回车键(Enter)] 开始录音...")
    
    print("录音中... 请说出你的唤醒词！")
    print("--> 说完后，按 [回车键(Enter)] 停止录音...")
    
    # Store audio data
    recording = []
    
    # Callback to stream audio
    def callback(indata, frames, time, status):
        if status:
            print(status, file=sys.stderr)
        recording.append(indata.copy())

    # Start non-blocking stream
    stream = sd.InputStream(samplerate=SAMPLE_RATE, channels=CHANNELS, dtype='int16', callback=callback)
    with stream:
        input() # Wait for Enter key to stop
    
    # Concatenate and save to WAV
    audio_data = np.concatenate(recording, axis=0)
    
    with wave.open(file_path, 'wb') as wf:
        wf.setnchannels(CHANNELS)
        wf.setsampwidth(2) # 16-bit is 2 bytes
        wf.setframerate(SAMPLE_RATE)
        wf.writeframes(audio_data.tobytes())
    
    print(f"录音已保存: {file_path} (时长: {len(audio_data)/SAMPLE_RATE:.2f}秒)")

def main():
    if not os.path.exists(OUTPUT_DIR):
        os.makedirs(OUTPUT_DIR)
        print(f"创建样例目录: {OUTPUT_DIR}")

    print("====================================================")
    # Highlight folder path using file scheme
    print(f"欢迎使用自定义唤醒词录音助手！")
    print(f"录音将以 16kHz 单声道 WAV 格式保存在 [my_samples](file://{os.path.abspath(OUTPUT_DIR)}) 目录中。")
    print("====================================================")

    # Find the next available index
    idx = 1
    while os.path.exists(os.path.join(OUTPUT_DIR, f"sample_{idx:02d}.wav")):
        idx += 1

    while True:
        file_name = f"sample_{idx:02d}.wav"
        file_path = os.path.join(OUTPUT_DIR, file_name)
        
        try:
            record_sample(file_path)
        except Exception as e:
            print(f"录音发生错误: {e}")
            break

        ans = input("\n选项: [回车] 录制下一个 | [r] 重新录制当前 | [q] 退出程序: ").strip().lower()
        if ans == 'q':
            print("\n录音结束！感谢使用。")
            break
        elif ans == 'r':
            print("准备重新录制当前样本...")
            if os.path.exists(file_path):
                os.remove(file_path)
        else:
            idx += 1

if __name__ == "__main__":
    main()
