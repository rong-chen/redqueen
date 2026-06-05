# RedQueen System (红皇后智能语音中枢)

本项目是一个全栈式本地智能语音交互控制中心（RedQueen）。包含 Vue 3 管理后台前端、Golang 高性能 WebSocket/API 后端，以及一套专门配套使用的离线唤醒词训练工具集（microWakeWord 炼丹炉）。

## 系统核心架构

1. **管理前端 (`admin-ui/`)**: 
   - 提供语音交互日志看板、MCP 服务接入控制。
   - **本地唤醒词采集录音机 (`/wakeword-recorder`)**: 纯前端实现，使用浏览器麦克风录制个人音频并强制重采样为 `16kHz WAV` 格式，支持一键打包下载。
   - **双重声纹管理 (`/voiceprint-enroll`)**: 用于录入主人的声纹，实现极高安全性的语音验证门禁。
2. **中枢后端 (`controllers/`, `services/`, `models/`)**: 
   - Golang 高并发 WebSocket 服务器，负责接收前端语音流。
   - 集成 `WeSpeaker` (基于 ONNX) 声纹识别，严格把关每一个语音请求，仅允许注册用户（如：主人）控制设备。
3. **离线唤醒词训练器 (`tools/wakeword-trainer/`)**:
   - 包含一键傻瓜式 Python 脚本 (`run_training.py`)，用于训练能在微控制器（如 ESP32-S3）及前端 `uww.js` 完美运行的极端抗噪唤醒模型（`.tflite`）。

---

## 🛠️ 如何组合使用：打造专属“红皇后”唤醒模型

这套系统的精髓在于**“傻瓜式前端录制” + “本地一键数据增强训练” + “多重验证拦截”**。请按照以下步骤，零代码生成您的私有极小巧（<1MB）抗噪唤醒模型！

### 第一步：采集您自己的语音
官方的开源库如果用合成声音，在真实环境里很容易误唤醒。用您自己真实的录音效果最好！
1. 启动项目，进入管理前端后台系统。
2. 点击左侧菜单的 **🎙️ 本地唤醒词采集录音机**。
3. 对着麦克风，以不同语速和距离录制 20~30 遍 “红皇后”。
4. 点击 **一键打包下载 (ZIP)**，您会得到一个 `custom_wakeword_samples.zip` 文件。

### 第二步：一键“炼丹”（训练模型）
拿到了干净的个人录音后，我们需要在本地为其叠加海量的真实世界底噪（电视声、马路声），让模型在复杂的现实环境中依然百发百中。
1. 在您的 Mac 或 Linux 机器上创建一个专用的 Python 虚拟环境，并安装所需底层库：
   ```bash
   # (建议在项目外单独建一个独立的环境文件夹进行训练，因为会下载几个 G 的噪音数据)
   python3 -m venv venv
   source venv/bin/activate
   pip install torch torchaudio
   pip install 'git+https://github.com/puddly/pymicro-features@puddly/minimum-cpp-version'
   pip install 'git+https://github.com/whatsnowplaying/audio-metadata@d4ebb238e6a401bb1a5aaaac60c9e2b3cb30929f'
   pip install -e 'git+https://github.com/kahrendt/microWakeWord.git#egg=microwakeword'
   ```
2. 将您刚才下载的 `custom_wakeword_samples.zip` 解压。
3. 在虚拟环境所在的目录下，新建一个 `custom_samples/` 文件夹，把解压出来的所有 `.wav` 录音扔进去。
4. 将本项目 `tools/wakeword-trainer/run_training.py` 脚本复制到您的虚拟环境目录中。
5. 确保脚本里 `use_custom_samples = True`。
6. 运行脚本：
   ```bash
   python3 run_training.py
   ```
7. 脚本会自动下载 MIT 环境噪音和 FMA 背景声，将您的 20 条录音裂变成几万条，经过 1~2 小时的模型训练后，生成专属的 `.tflite` 文件！

### 第三步：应用模型与双重防御
1. **本地极速唤醒**：将训练得到的 `.tflite` 文件加载入前端 Vue 页面的 `uww.js` 或者直接烧录进 ESP32-S3 设备中。这第一道防线只做极速唤醒。
2. **声纹看门狗拦截**：设备被唤醒后，音频流会通过 WebSocket 送入 Go 后端。后端底层的 `WeSpeaker` 声纹大模型会进行二次精准校验，如果识别出不是“主人”的声纹，无论说了什么都将直接切断通信，实现军工级的双重防御。
