<template>
  <div class="voice-terminal-container">
    <!-- 3D 全屏红皇后全息投影 -->
    <div class="hologram-canvas-container">
      <canvas ref="threeCanvasRef" class="hologram-canvas"></canvas>
      
      <!-- 生化危机/伞公司风格高科技 HUD 遮罩 -->
      <div class="hud-overlay">
        <div class="hud-corner top-left"></div>
        <div class="hud-corner top-right"></div>
        <div class="hud-corner bottom-left"></div>
        <div class="hud-corner bottom-right"></div>
        <div class="hud-grid-line"></div>
        
        <!-- 蜂巢/伞公司安全系统标识 -->
        <div class="hud-system-info">
          <div class="hud-logo-icon">▲</div>
          <div class="system-status-text">
            <div>SYSTEM: UMBRELLA CORP. HIVE SECURITY AI</div>
            <div>ACCESS LEVEL: ALPHA DECRYPTED (10)</div>
            <div>CODENAME: RED QUEEN (v4.02-RELEASE)</div>
            <div class="model-status-tag" :class="{ 'model-loaded': isModelLoaded }">
              MODEL: {{ isModelLoaded ? '3D GOTHIC RED LOLITA LOADED' : 'LOADING GOTHIC DIGITAL DOUBLE...' }}
            </div>
          </div>
        </div>

        <!-- 动态传感器模拟刻度 -->
        <div class="hud-scanners">
          <div class="hud-scanner-item">
            <span class="scanner-label">CPU LOAD</span>
            <span class="scanner-val">{{ cpuLoad }}%</span>
          </div>
          <div class="hud-scanner-item">
            <span class="scanner-label">MEM SCAN</span>
            <span class="scanner-val">ACTIVE</span>
          </div>
          <div class="hud-scanner-item">
            <span class="scanner-label">ROTATION</span>
            <span class="scanner-val">DRAG ENABLED</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 悬浮在高科技全息图之上的玻璃拟态交互面板 -->
    <div class="terminal-ui-overlay">
      <!-- 左侧悬浮：主控制板与识别显示 -->
      <div class="interactive-left-panel">
        <div class="glass-panel voice-control-card" :class="{ 'active': isActive, 'sleeping': !isActive }">
          <div class="panel-header">
            <span class="panel-title">
              <span class="status-dot" :class="statusDotClass"></span>
              {{ statusTitle }}
            </span>
            <span class="pulse-recording-tag" v-if="isRecording">
              <span class="pulse-ring-red"></span>
              <span class="pulse-text-red">正在聆听...</span>
            </span>
          </div>

          <!-- 实时识别文字显示 -->
          <div class="recognition-glass-area">
            <div class="recognition-label">实时识别指令</div>
            <div class="recognition-content" :class="{ 'content-partial': !isFinal }">
              {{ currentText || '静候您的呼唤... 说“皇后”唤醒系统' }}
            </div>
          </div>

          <!-- 2D 辅助频谱波形（麦克风有声音时显示） -->
          <div class="mini-waveform-container" v-show="isRecording">
            <canvas ref="canvasRef" class="waveform-canvas-mini"></canvas>
          </div>

          <!-- 控制按钮组 -->
          <div class="control-buttons-group">
            <button
              v-if="!isConnected"
              @click="startVoice"
              class="cyber-btn btn-primary"
            >
              <el-icon><Microphone /></el-icon>
              <span>初始化安全语音链路</span>
            </button>

            <template v-else>
              <button
                @click="toggleRecording"
                class="cyber-btn"
                :class="isRecording ? 'btn-danger' : 'btn-success'"
              >
                <el-icon>
                  <component :is="isRecording ? 'VideoPause' : 'Microphone'" />
                </el-icon>
                <span>{{ isRecording ? '关闭声音采集' : '恢复声音采集' }}</span>
              </button>

              <button
                @click="stopVoice"
                class="cyber-btn btn-secondary"
              >
                <span>断开链路</span>
              </button>
            </template>
          </div>
        </div>

        <!-- 新增：全息图控制台与本地模型导入 -->
        <div class="glass-panel hologram-settings-card" style="margin-top: 16px;">
          <div class="panel-header">
            <span class="panel-title">
              <el-icon><Setting /></el-icon>
              全息投影舱管理
            </span>
          </div>
          
          <div class="hologram-settings-body">
            <!-- 预设模型切换 -->
            <div class="setting-row">
              <span class="setting-label">投影角色预设</span>
              <el-radio-group v-model="selectedModelType" size="small" @change="changeModelType" class="cyber-radio-group">
                <el-radio-button label="default">预设萝莉</el-radio-button>
                <el-radio-button label="custom">自定义角色</el-radio-button>
                <el-radio-button label="core">智能星核</el-radio-button>
              </el-radio-group>
            </div>

            <!-- 本地模型导入区域 -->
            <div class="custom-model-importer" v-show="selectedModelType === 'custom'">
              <div 
                class="drag-drop-zone"
                :class="{ 'dragging': isDragOver }"
                @dragover.prevent="isDragOver = true"
                @dragleave.prevent="isDragOver = false"
                @drop.prevent="handleModelDrop"
                @click="triggerFileSelect"
              >
                <el-icon class="upload-icon"><UploadFilled /></el-icon>
                <div class="upload-text">
                  <span v-if="customModelFileName" style="font-weight: bold; color: #ff3366;">已导入: {{ customModelFileName }}</span>
                  <span v-else>拖拽或点击导入本地 <b>红黑色哥特裙</b> 3D 模型 (.glb / .vrm)</span>
                </div>
                <input 
                  type="file" 
                  ref="fileInputRef" 
                  accept=".glb,.vrm" 
                  style="display: none;" 
                  @change="handleFileSelect"
                />
              </div>
              
              <div class="importer-actions" v-if="customModelFileName">
                <button class="cyber-btn-mini btn-danger-mini" @click="clearCustomModel">清除自定义模型</button>
              </div>
              
              <div class="importer-tips">
                提示：请导入您自己喜爱的红黑色哥特裙角色模型，系统将利用骨骼适配器自动驱动其做出倾听、呼吸与 TTS 招手姿态。模型数据在本地 IndexedDB 永久安全保存。
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧悬浮：对话历史时间线 -->
      <div class="interactive-right-panel">
        <div class="glass-panel message-history-card">
          <div class="panel-header header-between">
            <div class="panel-title" style="display: flex; align-items: center; gap: 8px;">
              <el-icon><ChatLineRound /></el-icon>
              <span>通信日志记录</span>
            </div>
            <div class="history-actions">
              <el-select v-model="selectedVoiceName" size="small" placeholder="音色" style="width: 100px;" class="cyber-select" v-if="availableVoices.length > 0">
                <el-option
                  v-for="voice in availableVoices"
                  :key="voice.name"
                  :label="voice.name.replace('Microsoft', '微软').replace('Google', '谷歌')"
                  :value="voice.name"
                ></el-option>
              </el-select>
              <button class="cyber-btn-mini" @click="messages = []">清除日志</button>
            </div>
          </div>

          <div class="message-list" ref="messageListRef">
            <div v-if="messages.length === 0" class="empty-messages">
              <div class="no-logs-icon">▲</div>
              <p>暂无交互日志记录</p>
            </div>
            <TransitionGroup name="msg" tag="div">
              <div
                v-for="(msg, index) in messages"
                :key="index"
                class="message-item"
                :class="'msg-' + msg.type"
              >
                <div class="msg-icon-glow"></div>
                <div class="msg-body">
                  <div class="msg-content">
                    <span v-if="msg.text" class="msg-text">{{ msg.text }}</span>
                    <span v-if="msg.message" class="msg-desc">{{ msg.message }}</span>
                    <span
                      v-if="msg.intent"
                      class="msg-intent-tag"
                      :class="msg.status === 'success' ? 'tag-success' : 'tag-failed'"
                    >
                      {{ msg.intent }} | {{ msg.status }}
                    </span>
                  </div>
                  <div class="msg-time">{{ msg.time }}</div>
                </div>
              </div>
            </TransitionGroup>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick, computed, watch } from 'vue';
import { Microphone, VideoPause, ChatLineRound, Setting, UploadFilled } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import * as THREE from 'three';
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js';

// ---------------------------------------------------------------------------
// 状态与响应式变量
// ---------------------------------------------------------------------------
const isConnected = ref(false);
const isRecording = ref(false);
const isActive = ref(false);       // 皇后是否被唤醒
const currentText = ref('');       // 实时识别文字
const isFinal = ref(false);        // 当前文字是否为最终结果
const messages = ref([]);           // 交互消息历史
const availableVoices = ref([]);
const selectedVoiceName = ref('');
const cpuLoad = ref(24);            // 模拟高科技 HUD CPU 负载
const isModelLoaded = ref(false);   // 3D 骨骼模型是否加载完毕
const isSpeaking = ref(false);      // 红皇后本体当前是否正在播放语音答复
let currentStreamMessage = null;    // 当前正在接收的流式回复消息对象引用

// 监听朗读/播放状态，同步通知后端以避免 15s 自动休眠
watch(isSpeaking, (val) => {
  console.log('【ASR 升级】watch(isSpeaking) 触发，新值:', val);
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({
      type: 'speaking_status',
      text: val ? 'true' : 'false'
    }));
  }
});

// ---------------------------------------------------------------------------
// 🎨 自定义 3D 模型 IndexedDB 永久化本地保存工具组
// ---------------------------------------------------------------------------
const dbName = 'RedQueenHologramDB';
const storeName = 'models';
const modelKey = 'customModel';

function initDB() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(dbName, 1);
    request.onupgradeneeded = (e) => {
      const db = e.target.result;
      if (!db.objectStoreNames.contains(storeName)) {
        db.createObjectStore(storeName);
      }
    };
    request.onsuccess = (e) => resolve(e.target.result);
    request.onerror = (e) => reject(e.target.error);
  });
}

async function saveCustomModel(file) {
  const db = await initDB();
  return new Promise((resolve, reject) => {
    const transaction = db.transaction([storeName], 'readwrite');
    const store = transaction.objectStore(storeName);
    const request = store.put(file, modelKey);
    request.onsuccess = () => resolve();
    request.onerror = (e) => reject(e.target.error);
  });
}

async function getCustomModel() {
  const db = await initDB();
  return new Promise((resolve, reject) => {
    const transaction = db.transaction([storeName], 'readonly');
    const store = transaction.objectStore(storeName);
    const request = store.get(modelKey);
    request.onsuccess = (e) => resolve(e.target.result);
    request.onerror = (e) => reject(e.target.error);
  });
}

async function deleteCustomModel() {
  const db = await initDB();
  return new Promise((resolve, reject) => {
    const transaction = db.transaction([storeName], 'readwrite');
    const store = transaction.objectStore(storeName);
    const request = store.delete(modelKey);
    request.onsuccess = () => resolve();
    request.onerror = (e) => reject(e.target.error);
  });
}

// ---------------------------------------------------------------------------
// 萝莉模型设置与选择响应式变量
// ---------------------------------------------------------------------------
const selectedModelType = ref('default');
const customModelFileName = ref('');
const isDragOver = ref(false);
const fileInputRef = ref(null);

async function initModelConfig() {
  try {
    const savedType = localStorage.getItem('redqueen_model_type');
    if (savedType) {
      selectedModelType.value = savedType;
    }
    const savedName = localStorage.getItem('redqueen_custom_model_name');
    if (savedName) {
      customModelFileName.value = savedName;
    }
  } catch (err) {
    console.error('加载本地模型选项配置失败', err);
  }
}

// 智能检测并映射 3D 模型骨骼与表情变形器索引
function mapCharacterBonesAndMorphs(model) {
  // 重置缓存
  characterBones = {
    leftUpperArm: null,
    rightUpperArm: null,
    leftForearm: null,
    rightForearm: null,
    spine: null,
    neck: null,
    head: null
  };
  faceMorphs = {
    mesh: null,
    blinkIndex: -1,
    mouthIndex: -1
  };

  model.traverse((child) => {
    if (child.isBone) {
      const name = child.name.toLowerCase();
      
      // 判断左右侧的通用标识（支持 VRoid 标准、Mixamo、bip、Blender等）
      const isLeft = name.includes('left') || 
                     name.startsWith('l_') || 
                     name.startsWith('l.') || 
                     name.includes('_l_') || 
                     name.includes('.l.') || 
                     name.endsWith('_l') || 
                     name.endsWith('.l') || 
                     name.includes(' l ');
                     
      const isRight = name.includes('right') || 
                      name.startsWith('r_') || 
                      name.startsWith('r.') || 
                      name.includes('_r_') || 
                      name.includes('.r.') || 
                      name.endsWith('_r') || 
                      name.endsWith('.r') || 
                      name.includes(' r ');

      // 判断是否是手臂/手肘相关骨骼
      const isArm = name.includes('arm') || 
                    name.includes('forearm') || 
                    name.includes('lowerarm') || 
                    name.includes('elbow') || 
                    name === 'arm_l' || name === 'arm_r' || 
                    name === 'arm.l' || name === 'arm.r';

      // 判断是否是前臂/小臂/手肘（Lower）
      const isLower = name.includes('fore') || 
                      name.includes('lower') || 
                      name.includes('elbow') || 
                      name.includes('forearm') || 
                      name.includes('lowerarm');

      // 1. 左大臂 (Left Upper Arm)
      if (!characterBones.leftUpperArm && isLeft && isArm && !isLower && 
          !name.includes('hand') && !name.includes('finger') && !name.includes('wrist') && 
          !name.includes('palm') && !name.includes('clavicle') && !name.includes('shoulder')) {
        characterBones.leftUpperArm = child;
        console.log('Successfully mapped leftUpperArm to bone:', child.name);
      }
      
      // 2. 右大臂 (Right Upper Arm)
      else if (!characterBones.rightUpperArm && isRight && isArm && !isLower && 
               !name.includes('hand') && !name.includes('finger') && !name.includes('wrist') && 
               !name.includes('palm') && !name.includes('clavicle') && !name.includes('shoulder')) {
        characterBones.rightUpperArm = child;
        console.log('Successfully mapped rightUpperArm to bone:', child.name);
      }
      
      // 3. 左小臂 (Left Forearm)
      else if (!characterBones.leftForearm && isLeft && isArm && isLower &&
               !name.includes('hand') && !name.includes('finger') && !name.includes('wrist')) {
        characterBones.leftForearm = child;
        console.log('Successfully mapped leftForearm to bone:', child.name);
      }
      
      // 4. 右小臂 (Right Forearm)
      else if (!characterBones.rightForearm && isRight && isArm && isLower &&
               !name.includes('hand') && !name.includes('finger') && !name.includes('wrist')) {
        characterBones.rightForearm = child;
        console.log('Successfully mapped rightForearm to bone:', child.name);
      }
      
      // 5. 脊椎 (Spine)
      else if (!characterBones.spine && (
        name === 'spine' ||
        name === 'chest' ||
        name === 'upperchest' ||
        name.includes('spine') ||
        name.includes('chest') ||
        name.includes('mixamorigspine')
      )) {
        characterBones.spine = child;
        console.log('Successfully mapped spine to bone:', child.name);
      }
      
      // 6. 颈椎 (Neck)
      else if (!characterBones.neck && (
        name === 'neck' ||
        name.includes('neck') ||
        name.includes('mixamorigneck')
      )) {
        characterBones.neck = child;
        console.log('Successfully mapped neck to bone:', child.name);
      }
      
      // 7. 头部 (Head)
      else if (!characterBones.head && (
        name === 'head' ||
        name.includes('head') ||
        name.includes('mixamorighead')
      )) {
        characterBones.head = child;
        console.log('Successfully mapped head to bone:', child.name);
      }
    }
    
    // 自动扫描查找含有面部表情的网格 (Mesh)
    if (child.isMesh && child.morphTargetDictionary) {
      const dict = child.morphTargetDictionary;
      const keys = Object.keys(dict);
      
      let blinkIdx = -1;
      let mouthIdx = -1;
      
      for (let i = 0; i < keys.length; i++) {
        const key = keys[i].toLowerCase();
        
        // 查找眨眼变形通道
        if (blinkIdx === -1 && (
          key === 'blink' ||
          key === 'eye_close' ||
          key === 'eyeclosed' ||
          key.includes('blink') ||
          (key.includes('eye') && (key.includes('close') || key.includes('shut'))) ||
          key.includes('close_l') || key.includes('close_r') ||
          key.includes('fcl_eye_close')
        )) {
          blinkIdx = dict[keys[i]];
        }
        
        // 查找张嘴发音 A 通道
        if (mouthIdx === -1 && (
          key === 'mouth_open' ||
          key === 'jaw_open' ||
          key === 'mouth_a' ||
          key === 'vowel_a' ||
          key === 'shape_a' ||
          key === 'a' ||
          key === 'ah' ||
          key.includes('mouth_open') ||
          key.includes('jaw_open') ||
          key.includes('mouth_a') ||
          key.includes('fcl_mth_o') ||
          key.includes('fcl_mth_a') ||
          key.includes('vowel_a')
        )) {
          mouthIdx = dict[keys[i]];
        }
      }
      
      // 选择包含眨眼或张嘴、或者表情选项最多的 Mesh 作为表情承载器
      if (blinkIdx !== -1 || mouthIdx !== -1 || (faceMorphs.mesh === null && keys.length > 8)) {
        if (!faceMorphs.mesh || keys.length > Object.keys(faceMorphs.mesh.morphTargetDictionary).length) {
          faceMorphs.mesh = child;
          faceMorphs.blinkIndex = blinkIdx;
          faceMorphs.mouthIndex = mouthIdx;
          console.log('Successfully mapped faceMesh to:', child.name, 'with blinkIdx:', blinkIdx, 'mouthIdx:', mouthIdx);
        }
      }
    }
  });
}

function setupLoadedModel(model) {
  // 清理老角色模型
  if (characterModel) {
    hologramGroup.remove(characterModel);
    characterModel.traverse((child) => {
      if (child.isMesh) {
        if (child.geometry) child.geometry.dispose();
        if (child.material) {
          if (Array.isArray(child.material)) {
            child.material.forEach(m => m.dispose());
          } else {
            child.material.dispose();
          }
        }
      }
    });
    characterModel = null;
  }
  // 清理备用核心
  if (outerMesh) {
    hologramGroup.remove(outerMesh);
    outerMesh.geometry.dispose();
    outerMesh.material.dispose();
    outerMesh = null;
  }

  characterModel = model;

  // 缩放和位置自适应
  characterModel.scale.set(1.5, 1.5, 1.5);
  characterModel.position.set(0, -1.8, 0);

  // 解析并缓存新角色的骨骼与 Morph Targets 表情索引
  mapCharacterBonesAndMorphs(characterModel);

  applyCharacterMeshStyling(0);

  hologramGroup.add(characterModel);
  isModelLoaded.value = true;
}

function setupFallbackCore() {
  if (characterModel) {
    hologramGroup.remove(characterModel);
    characterModel.traverse((child) => {
      if (child.isMesh) {
        if (child.geometry) child.geometry.dispose();
        if (child.material) {
          if (Array.isArray(child.material)) {
            child.material.forEach(m => m.dispose());
          } else {
            child.material.dispose();
          }
        }
      }
    });
    characterModel = null;
  }
  if (outerMesh) return; // 已经存在

  const outerGeom = new THREE.IcosahedronGeometry(1.6, 2);
  const outerMat = new THREE.MeshBasicMaterial({
    color: 0xff0033,
    wireframe: true,
    transparent: true,
    opacity: 0.25,
    blending: THREE.AdditiveBlending
  });
  outerMesh = new THREE.Mesh(outerGeom, outerMat);
  hologramGroup.add(outerMesh);
  isModelLoaded.value = true;
}

async function triggerModelLoading() {
  isModelLoaded.value = false;
  
  if (selectedModelType.value === 'core') {
    setupFallbackCore();
    return;
  }
  
  if (selectedModelType.value === 'custom') {
    try {
      const customFile = await getCustomModel();
      if (customFile) {
        const loader = new GLTFLoader();
        const objectUrl = URL.createObjectURL(customFile);
        loader.load(
          objectUrl,
          (gltf) => {
            setupLoadedModel(gltf.scene);
            URL.revokeObjectURL(objectUrl);
            ElMessage.success(`自定义模型 ${customModelFileName.value} 加载成功`);
          },
          undefined,
          (err) => {
            console.error('加载自定义模型失败:', err);
            ElMessage.error('解析自定义模型文件失败，已降级至系统预设');
            URL.revokeObjectURL(objectUrl);
            selectedModelType.value = 'default';
            localStorage.setItem('redqueen_model_type', 'default');
            triggerModelLoading();
          }
        );
        return;
      }
    } catch (err) {
      console.error('获取本地自定义模型文件失败:', err);
    }
    // 如果无自定义模型，降级到默认
    selectedModelType.value = 'default';
    localStorage.setItem('redqueen_model_type', 'default');
  }

  // 加载系统预设
  const loader = new GLTFLoader();
  const modelUrl = 'https://cdn.jsdelivr.net/gh/masaaki-imai/3D-anime@main/public/3d/kawaii22.glb';
  loader.load(
    modelUrl,
    (gltf) => {
      setupLoadedModel(gltf.scene);
    },
    undefined,
    (err) => {
      console.error('3D 动漫女孩模型加载出错，降级采用全息智能球体', err);
      ElMessage.warning('全息投影加载受阻，系统已切入备用核心');
      setupFallbackCore();
    }
  );
}

// ---------------------------------------------------------------------------
// 3D 本地文件导入处理器
// ---------------------------------------------------------------------------
const triggerFileSelect = () => {
  if (fileInputRef.value) {
    fileInputRef.value.click();
  }
};

const handleFileSelect = (e) => {
  const files = e.target.files;
  if (files && files.length > 0) {
    importModelFile(files[0]);
  }
};

const handleModelDrop = (e) => {
  isDragOver.value = false;
  const files = e.dataTransfer.files;
  if (files && files.length > 0) {
    importModelFile(files[0]);
  }
};

async function importModelFile(file) {
  const name = file.name;
  if (!name.endsWith('.glb') && !name.endsWith('.vrm')) {
    ElMessage.error('不支持的文件格式！请导入 .glb 或 .vrm 格式的 3D 模型');
    return;
  }

  try {
    customModelFileName.value = name;
    localStorage.setItem('redqueen_custom_model_name', name);
    
    await saveCustomModel(file);
    ElMessage.success('自定义 3D 哥特萝莉模型本地保存成功');
    
    selectedModelType.value = 'custom';
    localStorage.setItem('redqueen_model_type', 'custom');
    triggerModelLoading();
  } catch (err) {
    console.error('导入模型文件失败:', err);
    ElMessage.error('本地保存模型失败，浏览器存储空间可能不足');
  }
}

async function clearCustomModel() {
  try {
    await deleteCustomModel();
    customModelFileName.value = '';
    localStorage.removeItem('redqueen_custom_model_name');
    
    if (selectedModelType.value === 'custom') {
      selectedModelType.value = 'default';
      localStorage.setItem('redqueen_model_type', 'default');
      triggerModelLoading();
    }
    ElMessage.success('自定义 3D 模型已清除');
  } catch (err) {
    console.error('清除模型文件失败:', err);
    ElMessage.error('清除模型失败');
  }
}

function changeModelType() {
  localStorage.setItem('redqueen_model_type', selectedModelType.value);
  triggerModelLoading();
}

// ---------------------------------------------------------------------------
// DOM 元素引用
// ---------------------------------------------------------------------------
const canvasRef = ref(null);
const threeCanvasRef = ref(null);
const messageListRef = ref(null);

// ---------------------------------------------------------------------------
// 原生多媒体与 WebRTC 状态
// ---------------------------------------------------------------------------
let ws = null;                      // WebSocket 实例
let audioContext = null;            // Web Audio API 上下文
let mediaStream = null;             // 麦克风媒体流
let scriptProcessor = null;         // 音频处理节点
let source = null;                  // 音频源节点
let analyser = null;                // 频谱分析节点
let animationId = null;             // 2D 波形动画帧 ID

// ---------------------------------------------------------------------------
// Three.js 状态与变量
// ---------------------------------------------------------------------------
let threeScene = null;
let threeCamera = null;
let threeRenderer = null;
let hologramGroup = null;
let coreMesh = null;
let outerMesh = null;
let particleSystem = null;
let scanLine = null;
const rings = [];
const soundWaves = [];
let threeAnimId = null;
let hudInterval = null;

// 3D 骨骼与角色控制
let characterModel = null;

// 3D 骨骼与角色控制缓存（缓存骨骼引用，完美兼容各种动漫骨骼命名）
let characterBones = {
  leftUpperArm: null,
  rightUpperArm: null,
  leftForearm: null,
  rightForearm: null,
  spine: null,
  neck: null,
  head: null
};

// 3D 表情/口型网格与索引缓存
let faceMorphs = {
  mesh: null,
  blinkIndex: -1,
  mouthIndex: -1
};

// 眨眼动画控制变量
let lastBlinkTime = 0;
let nextBlinkInterval = 3.0;
let blinkProgress = -1;
const blinkDuration = 0.15; // 眨眼动画总时长 (秒)

// ---------------------------------------------------------------------------
// 鼠标/触控拖拽左右滑动控制变量
// ---------------------------------------------------------------------------
let isDragging = false;
let previousMouseX = 0;
let targetRotationY = 0;  // 目标旋转角度，默认为 0（正面面对屏幕）
let currentRotationY = 0; // 当前平滑过渡中的旋转角度

const handleMouseDown = (e) => {
  // 排除点击在交互卡片、控制台按钮、下拉选择框等有特定逻辑的UI区域
  const path = e.composedPath ? e.composedPath() : [];
  const isInteractive = path.some(el => {
    if (!el || !el.classList) return false;
    return el.tagName === 'BUTTON' || 
           el.tagName === 'INPUT' || 
           el.tagName === 'SELECT' || 
           el.classList.contains('cyber-btn') || 
           el.classList.contains('glass-panel') ||
           el.classList.contains('interactive-left-panel') ||
           el.classList.contains('interactive-right-panel') ||
           el.classList.contains('cyber-btn-mini') ||
           el.classList.contains('cyber-select');
  });
  if (isInteractive) return;

  isDragging = true;
  previousMouseX = e.clientX;
  
  // 改变鼠标指针样式
  const canvas = threeCanvasRef.value;
  if (canvas) canvas.style.cursor = 'grabbing';
};

const handleMouseMove = (e) => {
  if (!isDragging) return;
  const deltaX = e.clientX - previousMouseX;
  previousMouseX = e.clientX;
  
  // 左右滑动调节旋转目标值 (0.007控制滑动阻尼和感度)
  targetRotationY += deltaX * 0.007;
};

const handleMouseUp = () => {
  isDragging = false;
  const canvas = threeCanvasRef.value;
  if (canvas) canvas.style.cursor = 'grab';
};

const handleTouchStart = (e) => {
  if (e.touches.length === 1) {
    const path = e.composedPath ? e.composedPath() : [];
    const isInteractive = path.some(el => {
      if (!el || !el.classList) return false;
      return el.tagName === 'BUTTON' || 
             el.tagName === 'INPUT' || 
             el.tagName === 'SELECT' || 
             el.classList.contains('cyber-btn') || 
             el.classList.contains('glass-panel') ||
             el.classList.contains('interactive-left-panel') ||
             el.classList.contains('interactive-right-panel') ||
             el.classList.contains('cyber-btn-mini') ||
             el.classList.contains('cyber-select');
    });
    if (isInteractive) return;

    isDragging = true;
    previousMouseX = e.touches[0].clientX;
  }
};

const handleTouchMove = (e) => {
  if (!isDragging || e.touches.length !== 1) return;
  const deltaX = e.touches[0].clientX - previousMouseX;
  previousMouseX = e.touches[0].clientX;
  targetRotationY += deltaX * 0.007;
};

// ---------------------------------------------------------------------------
// 计算属性
// ---------------------------------------------------------------------------
const statusTitle = computed(() => {
  if (!isConnected.value) return '系统离线';
  if (isActive.value) return '红皇后·高度警戒中';
  return '红皇后·静默监听中';
});

const statusDotClass = computed(() => ({
  'dot-active': isConnected.value && isActive.value,
  'dot-sleeping': isConnected.value && !isActive.value,
  'dot-disconnected': !isConnected.value,
}));

// ---------------------------------------------------------------------------
// 系统音色加载与本地播放
// ---------------------------------------------------------------------------
function loadVoices() {
  if (typeof window !== 'undefined' && window.speechSynthesis) {
    const allVoices = window.speechSynthesis.getVoices();
    availableVoices.value = allVoices.filter(v => v.lang.startsWith('zh') || v.lang.includes('ZH'));
    
    if (availableVoices.value.length > 0 && !selectedVoiceName.value) {
      const defaultVoice = availableVoices.value.find(v => 
        v.name.includes('Siri') || v.name.includes('Tingting') || v.name.includes('Xiaoxiao') || v.name.includes('Google') || v.name.includes('Microsoft')
      ) || availableVoices.value[0];
      selectedVoiceName.value = defaultVoice.name;
    }
  }
}

function speakText(text) {
  if (!text) return;
  try {
    window.speechSynthesis.cancel();

    const utterance = new SpeechSynthesisUtterance(text);
    utterance.lang = 'zh-CN';
    utterance.rate = 1.05;
    utterance.pitch = 0.95; // 略微低沉清冷的音色，符合红皇后性格设定

    const voices = window.speechSynthesis.getVoices();
    const targetVoice = voices.find(v => v.name === selectedVoiceName.value);
    if (targetVoice) {
      utterance.voice = targetVoice;
    } else {
      const fallbackVoice = voices.find(v => v.lang.startsWith('zh'));
      if (fallbackVoice) utterance.voice = fallbackVoice;
    }

    // 绑定开始与结束事件，精确同步说话动作状态机！
    utterance.onstart = () => {
      isSpeaking.value = true;
    };
    utterance.onend = () => {
      isSpeaking.value = false;
    };
    utterance.onerror = () => {
      isSpeaking.value = false;
    };
    utterance.onpause = () => {
      isSpeaking.value = false;
    };

    window.speechSynthesis.speak(utterance);
  } catch (err) {
    console.error('TTS 播放失败:', err);
    isSpeaking.value = false;
  }
}

// ---------------------------------------------------------------------------
// 🎙️ 服务端高保真 Edge Neural TTS 实时二进制音频流播放器
// ---------------------------------------------------------------------------
let activeAudioSources = [];
let nextPlaybackTime = 0;

function ensureAudioContext() {
  if (!audioContext) {
    audioContext = new (window.AudioContext || window.webkitAudioContext)({
      sampleRate: 16000
    });
  }
  if (!analyser) {
    analyser = audioContext.createAnalyser();
    analyser.fftSize = 256;
  }
  if (audioContext.state === 'suspended') {
    audioContext.resume();
  }
}

function playBinaryPCM(arrayBuffer) {
  try {
    ensureAudioContext();
    
    // 后端默认推送的是 16kHz 16-bit 单声道 Little-Endian PCM 原始音频帧
    const rawData = new Int16Array(arrayBuffer);
    if (rawData.length === 0) return;
    
    // 将 int16 采样归一化至 [-1.0, 1.0] 的浮点数以便 Web Audio API 播放
    const floatData = new Float32Array(rawData.length);
    for (let i = 0; i < rawData.length; i++) {
      floatData[i] = rawData[i] / 32768.0;
    }
    
    // 创建一个单声道音频缓冲区，采样率为固定的 16000Hz
    const buffer = audioContext.createBuffer(1, floatData.length, 16000);
    buffer.copyToChannel(floatData, 0);
    
    const sourceNode = audioContext.createBufferSource();
    sourceNode.buffer = buffer;
    
    // 🔗 完美闭环：将播放节点连接到全局频谱分析器 (analyser)
    // 这使得前端已有的 2D 波形频谱、3D 星核跳动、面部 Morph 语音张嘴开合会自动完美跟随声音实时律动！
    sourceNode.connect(analyser);
    sourceNode.connect(audioContext.destination); // 输出到系统默认扬声器/耳机
    
    // 使用排队调度算法，实现跨网络分包的无缝、平滑、零爆音衔接播放 (LERP-like time queueing)
    const startTime = Math.max(audioContext.currentTime, nextPlaybackTime);
    sourceNode.start(startTime);
    
    // 累加下一次排队播放时间
    nextPlaybackTime = startTime + buffer.duration;
    
    activeAudioSources.push(sourceNode);
    
    // 播放完毕的垃圾回收与状态机切换
    sourceNode.onended = () => {
      const idx = activeAudioSources.indexOf(sourceNode);
      if (idx !== -1) {
        activeAudioSources.splice(idx, 1);
      }
      // 当全部缓冲区流式包播放完毕，且当前处于说话姿态时
      if (activeAudioSources.length === 0 && isSpeaking.value) {
        setTimeout(() => {
          if (activeAudioSources.length === 0) {
            isSpeaking.value = false; // 平滑切换回闲置背手待机姿势
          }
        }, 150);
      }
    };
  } catch (err) {
    console.error('【TTS 流式播放】解析音频帧失败:', err);
  }
}

function stopAllAudioPlayback() {
  activeAudioSources.forEach(src => {
    try {
      src.stop();
    } catch (e) {}
  });
  activeAudioSources = [];
  nextPlaybackTime = 0;
  isSpeaking.value = false;
}

// ---------------------------------------------------------------------------
// WebSocket 联络
// ---------------------------------------------------------------------------
function getWSUrl() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//localhost:9091/api/voice/ws`;
}

function connectWebSocket() {
  return new Promise((resolve, reject) => {
    const url = getWSUrl();
    ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer'; // 启用原始 ArrayBuffer 二进制接收通道

    ws.onopen = () => {
      isConnected.value = true;
      addMessage('system', { message: '安全通信链路已建立' });
      resolve();
    };

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        handleServerMessage(JSON.parse(event.data));
      } else if (event.data instanceof ArrayBuffer) {
        // 收到服务端 Edge Neural TTS 极速流式分包音频，直接进行 Web Audio 高保真对齐播送！
        playBinaryPCM(event.data);
      }
    };

    ws.onclose = () => {
      isConnected.value = false;
      isActive.value = false;
      isRecording.value = false;
      stopAllAudioPlayback();
      addMessage('system', { message: '安全通信链路断开' });
    };

    ws.onerror = (err) => {
      console.error('WebSocket error:', err);
      ElMessage.error('通信链路连接失败，请检查控制网关是否启动');
      reject(err);
    };
  });
}

function handleServerMessage(msg) {
  switch (msg.type) {
    case 'ready':
      addMessage('system', msg);
      break;

    case 'partial':
      currentText.value = msg.text;
      isFinal.value = false;
      break;

    case 'final':
      currentText.value = msg.text;
      isFinal.value = true;
      addMessage('final', msg);
      break;

    case 'wake':
      isActive.value = true;
      currentStreamMessage = null; // 清除流式缓存
      addMessage('wake', msg);
      break;

    case 'sleep':
      isActive.value = false;
      currentStreamMessage = null; // 清除流式缓存
      addMessage('sleep', msg);
      break;

    case 'voiceprint_blocked':
      addMessage('error', { text: msg.text || '', message: `【安全声纹锁拦截】${msg.message}` });
      ElMessage.warning(`【声纹安全拦截】${msg.message}`);
      break;

    case 'stream_token':
      // 收到大模型流式回复分片，追加到当前通信气泡中，避免碎片化
      if (!currentStreamMessage) {
        const now = new Date();
        const time = now.toLocaleTimeString('zh-CN', { hour12: false });
        const newMsgObj = {
          type: 'result',
          text: '',
          message: msg.message || '',
          intent: 'conversation',
          status: 'pending',
          time,
        };
        messages.value.push(newMsgObj);
        // 获取 Vue 包装后的响应式代理对象，以便后续修改能触发 UI 更新
        currentStreamMessage = messages.value[messages.value.length - 1];
      } else {
        currentStreamMessage.message += (msg.message || '');
      }

      nextTick(() => {
        const list = messageListRef.value;
        if (list) {
          list.scrollTop = list.scrollHeight;
        }
      });
      break;

    case 'result':
      if (currentStreamMessage) {
        currentStreamMessage.intent = msg.intent || 'conversation';
        currentStreamMessage.status = msg.status || 'success';
        if (msg.message) {
          currentStreamMessage.message = msg.message; // 确保使用后端的最终完整文本覆盖
        }
        currentStreamMessage = null; // 结束流式气泡编辑状态
      } else {
        addMessage('result', msg);
      }
      break;

    case 'audio_start':
      // 接收到音频流开始传输信号，做全局音频上下文就绪检测
      isSpeaking.value = true;
      stopAllAudioPlayback();
      nextPlaybackTime = 0; // 重置排队播放时钟轴
      break;

    case 'audio_end':
      // 音频流传输完毕（具体关闭由播放完后的 activeAudioSources.length === 0 平滑完成）
      break;

    case 'interrupt':
      if (typeof window !== 'undefined' && window.speechSynthesis) {
        window.speechSynthesis.cancel();
      }
      // 立即打断并清除当前正在播放的所有音频流，实现秒级快速打断响应！
      stopAllAudioPlayback();
      currentStreamMessage = null; // 打断时清空流式气泡缓存
      addMessage('system', { message: '语音流被打断，接收全新指令' });
      break;

    default:
      addMessage('system', msg);
  }
}

// ---------------------------------------------------------------------------
// 麦克风输入与音频管线
// ---------------------------------------------------------------------------
async function startMicrophone() {
  try {
    mediaStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        sampleRate: 16000,
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
      },
    });

    audioContext = new (window.AudioContext || window.webkitAudioContext)({
      sampleRate: 16000,
    });

    source = audioContext.createMediaStreamSource(mediaStream);
    analyser = audioContext.createAnalyser();
    analyser.fftSize = 256;
    source.connect(analyser);

    scriptProcessor = audioContext.createScriptProcessor(4096, 1, 1);
    scriptProcessor.onaudioprocess = (event) => {
      if (!isRecording.value || !ws || ws.readyState !== WebSocket.OPEN) return;

      const inputData = event.inputBuffer.getChannelData(0);
      const pcmData = new Int16Array(inputData.length);
      for (let i = 0; i < inputData.length; i++) {
        const s = Math.max(-1, Math.min(1, inputData[i]));
        pcmData[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
      }
      ws.send(pcmData.buffer);
    };

    source.connect(scriptProcessor);
    scriptProcessor.connect(audioContext.destination);

    start2DWaveformAnimation();
    isRecording.value = true;
  } catch (err) {
    console.error('麦克风采集失败:', err);
    ElMessage.error('无法激活音频采集设备，请授予麦克风权限');
    throw err;
  }
}

function stopMicrophone() {
  isRecording.value = false;

  if (animationId) {
    cancelAnimationFrame(animationId);
    animationId = null;
  }
  if (scriptProcessor) {
    scriptProcessor.disconnect();
    scriptProcessor = null;
  }
  if (source) {
    source.disconnect();
    source = null;
  }
  if (analyser) {
    analyser.disconnect();
    analyser = null;
  }
  if (audioContext) {
    audioContext.close();
    audioContext = null;
  }
  if (mediaStream) {
    mediaStream.getTracks().forEach(track => track.stop());
    mediaStream = null;
  }

  const canvas = canvasRef.value;
  if (canvas) {
    const ctx = canvas.getContext('2d');
    ctx.clearRect(0, 0, canvas.width, canvas.height);
  }
}

// ---------------------------------------------------------------------------
// 2D 辅助频谱波形（绘制于左下角）
// ---------------------------------------------------------------------------
function start2DWaveformAnimation() {
  const canvas = canvasRef.value;
  if (!canvas || !analyser) return;

  const ctx = canvas.getContext('2d');
  const bufferLength = analyser.frequencyBinCount;
  const dataArray = new Uint8Array(bufferLength);

  function draw() {
    animationId = requestAnimationFrame(draw);

    canvas.width = canvas.offsetWidth * (window.devicePixelRatio || 1);
    canvas.height = canvas.offsetHeight * (window.devicePixelRatio || 1);
    ctx.scale(window.devicePixelRatio || 1, window.devicePixelRatio || 1);

    const WIDTH = canvas.offsetWidth;
    const HEIGHT = canvas.offsetHeight;

    analyser.getByteFrequencyData(dataArray);

    ctx.fillStyle = 'rgba(0, 0, 0, 0)';
    ctx.clearRect(0, 0, WIDTH, HEIGHT);

    const barWidth = (WIDTH / bufferLength) * 1.5;
    let x = 0;

    for (let i = 0; i < bufferLength; i++) {
      const barHeight = (dataArray[i] / 255) * HEIGHT * 0.7;
      ctx.fillStyle = `rgba(255, 0, 85, ${0.3 + (dataArray[i] / 255) * 0.7})`;

      const barY = (HEIGHT - barHeight) / 2;
      const radius = Math.min(barWidth / 2, 2);
      
      ctx.beginPath();
      ctx.roundRect(x, barY, barWidth, barHeight, radius);
      ctx.fill();

      x += barWidth + 1;
    }
  }

  draw();
}

// ---------------------------------------------------------------------------
// 🎨 红黑哥特裙与白皙肤色精细化全息着色器材质分配器
// ---------------------------------------------------------------------------
function applyCharacterMeshStyling(audioVolume = 0) {
  if (!characterModel) return;

  characterModel.traverse((child) => {
    if (child.isMesh && child.material) {
      const name = child.name.toLowerCase();
      const matName = child.material.name ? child.material.name.toLowerCase() : '';
      
      const isSkin = name.includes('face') || name.includes('skin') || name.includes('body') || name.includes('eye') || 
                     matName.includes('skin') || matName.includes('face') || matName.includes('eye') || matName.includes('mouth');
      const isHair = name.includes('hair') || matName.includes('hair');
      
      child.material.wireframe = false;
      child.material.transparent = true;
      child.material.side = THREE.DoubleSide;
      child.material.depthWrite = true;
      
      if (isSkin) {
        // 肤色/眼睛：还原白皙肤色与清灵大眼，录音时微红
        child.material.opacity = 0.96 + audioVolume * 0.03;
        if (child.material.emissive) {
          if (isRecording.value) {
            child.material.emissive.setHex(0x2a0d15); // 聆听时稍微有粉红全息呼吸
          } else if (isSpeaking.value) {
            child.material.emissive.setHex(0x22050a); // 说话时粉红全息
          } else {
            child.material.emissive.setHex(0x120306); // 闲置时极淡粉红全息自发光，几乎不显色
          }
        }
      } else if (isHair) {
        // 红色双马尾头发：设置深红色自发光，与黑色搭配极佳
        child.material.opacity = 0.92 + audioVolume * 0.05;
        if (child.material.emissive) {
          if (isRecording.value) {
            child.material.emissive.setHex(0x660015);
          } else if (isSpeaking.value) {
            child.material.emissive.setHex(0x550011);
          } else {
            child.material.emissive.setHex(0x330008);
          }
        }
      } else {
        // 华丽的红黑色哥特裙（以及蕾丝鞋子等配饰）：
        // 设置极微弱的深红底色自发光，完美突显原材质中的红绸缎与黑蕾丝纹路细节，绝不洗白！
        child.material.opacity = 0.90 + audioVolume * 0.07;
        if (child.material.emissive) {
          if (isRecording.value) {
            child.material.emissive.setHex(0x4a000c);
          } else if (isSpeaking.value) {
            child.material.emissive.setHex(0x3d000a);
          } else {
            child.material.emissive.setHex(0x220005); // 闲置时微弱红发光，绝不掩盖红黑裙纹路细节
          }
        }
      }
    }
  });
}

// ---------------------------------------------------------------------------
// Three.js 3D 全屏红皇后全息引擎与骨骼动力学控制器
// ---------------------------------------------------------------------------
function initThreeHologram() {
  const canvas = threeCanvasRef.value;
  if (!canvas) return;

  const width = canvas.clientWidth;
  const height = canvas.clientHeight;

  // Scene
  threeScene = new THREE.Scene();
  threeScene.fog = new THREE.FogExp2(0x0a0005, 0.045);

  // Camera
  threeCamera = new THREE.PerspectiveCamera(45, width / height, 0.1, 100);
  threeCamera.position.set(0, -0.2, 5.5);

  // Renderer
  threeRenderer = new THREE.WebGLRenderer({
    canvas: canvas,
    antialias: true,
    alpha: false
  });
  threeRenderer.setSize(width, height);
  threeRenderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));

  // Hologram Group
  hologramGroup = new THREE.Group();
  threeScene.add(hologramGroup);

  // 1. Core Neural
  const coreGeom = new THREE.SphereGeometry(0.3, 16, 16);
  const coreMat = new THREE.MeshBasicMaterial({
    color: 0xff0044,
    wireframe: true,
    transparent: true,
    opacity: 0.12,
    blending: THREE.AdditiveBlending
  });
  coreMesh = new THREE.Mesh(coreGeom, coreMat);
  hologramGroup.add(coreMesh);

  // 初始化持久化模型配置，并启动加载
  initModelConfig().then(() => {
    triggerModelLoading();
  });

  // 3. 科技运行光环 (Orbit Rings)
  const ringMat = new THREE.MeshBasicMaterial({
    color: 0xff0033,
    wireframe: true,
    transparent: true,
    opacity: 0.18,
    blending: THREE.AdditiveBlending
  });

  const ring1 = new THREE.Mesh(new THREE.TorusGeometry(2.3, 0.012, 8, 64), ringMat);
  ring1.rotation.x = Math.PI / 2;
  hologramGroup.add(ring1);
  rings.push({ mesh: ring1, speedX: 0.001, speedY: 0.003 });

  const ring2 = new THREE.Mesh(new THREE.TorusGeometry(2.6, 0.008, 6, 48), ringMat);
  ring2.rotation.y = Math.PI / 4;
  hologramGroup.add(ring2);
  rings.push({ mesh: ring2, speedX: -0.002, speedY: 0.001 });

  // 4. 数字尘埃粒子 (Points)
  const particleCount = 550;
  const particleGeom = new THREE.BufferGeometry();
  const positions = new Float32Array(particleCount * 3);
  const velocities = [];

  for (let i = 0; i < particleCount; i++) {
    const theta = Math.random() * Math.PI * 2;
    const radius = 1.8 + Math.random() * 2.8;
    const x = Math.cos(theta) * radius;
    const y = (Math.random() - 0.5) * 6.5;
    const z = Math.sin(theta) * radius;

    positions[i * 3] = x;
    positions[i * 3 + 1] = y;
    positions[i * 3 + 2] = z;

    velocities.push({
      y: 0.004 + Math.random() * 0.01,
      angle: Math.random() * Math.PI * 2,
      angleSpeed: 0.004 + Math.random() * 0.008,
      radius: radius
    });
  }

  particleGeom.setAttribute('position', new THREE.BufferAttribute(positions, 3));
  const particleMat = new THREE.PointsMaterial({
    color: 0xff1e6c,
    size: 0.03,
    transparent: true,
    opacity: 0.4,
    blending: THREE.AdditiveBlending
  });
  particleSystem = new THREE.Points(particleGeom, particleMat);
  threeScene.add(particleSystem);

  // 5. 激光横向扫描面
  const scanLineGeom = new THREE.CylinderGeometry(2.8, 2.8, 0.03, 32, 1, true);
  const scanMat = new THREE.MeshBasicMaterial({
    color: 0xff0044,
    wireframe: true,
    transparent: true,
    opacity: 0.18,
    blending: THREE.AdditiveBlending
  });
  scanLine = new THREE.Mesh(scanLineGeom, scanMat);
  scanLine.position.y = -3;
  threeScene.add(scanLine);

  // 6. 灯光系统
  const ambLight = new THREE.AmbientLight(0xfff5f5, 1.3); // 温柔的暖白光作为底色环境光，完美显现衣服红黑色彩与皮肤细节
  threeScene.add(ambLight);

  const mainLight = new THREE.DirectionalLight(0xffffff, 3.5); // 强力主方向光，打在前方增加立体阴影与反射质感
  mainLight.position.set(2, 4, 5);
  threeScene.add(mainLight);

  const fillLight = new THREE.DirectionalLight(0xff0044, 2.8); // 侧向红光补光，打造完美的红色全息边缘轮廓RimLight效果
  fillLight.position.set(-3, 1, 3);
  threeScene.add(fillLight);

  const topLight = new THREE.PointLight(0xffffff, 1.8, 10); // 顶光源白光，为头发打上亮丽的发梢光斑
  topLight.position.set(0, 3, 0);
  threeScene.add(topLight);

  // 音波震荡圈通用材质
  const waveMat = new THREE.MeshBasicMaterial({
    color: 0xff0055,
    transparent: true,
    opacity: 0.8,
    wireframe: true,
    blending: THREE.AdditiveBlending
  });

  const clock = new THREE.Clock();
  let scanDir = 1;

  // ---------------------------------------------------------------------------
  // 🎭 动态多姿态体态机（Idle Poses State Machine）
  // ---------------------------------------------------------------------------
  const idlePoses = {
    // 姿态0：静谧乖巧（双手自然交叠垂于裙前两侧）
    relaxed: {
      l_arm: { x: 0, y: 0.08, z: -1.38 },
      r_arm: { x: 0, y: -0.08, z: 1.38 },
      l_fore: { x: 0, y: 0.18, z: 0 },
      r_fore: { x: 0, y: -0.18, z: 0 },
      spine: { x: 0, y: 0, z: 0 },
      neck: { x: 0.15, y: 0, z: 0.04 }
    },
    // 姿态1：俏皮好奇（双手背在腰后，身体微微前倾，偏着头端详您）
    handsBehind: {
      l_arm: { x: -0.45, y: -0.35, z: -1.45 },
      r_arm: { x: -0.45, y: 0.35, z: 1.45 },
      l_fore: { x: 0, y: 1.15, z: 0 },
      r_fore: { x: 0, y: -1.15, z: 0 },
      spine: { x: 0.04, y: 0, z: 0 },
      neck: { x: 0.14, y: 0.08, z: -0.04 }
    },
    // 姿态2：害羞腼腆（左手轻轻提起置于胸前心房处，右手自然垂落）
    handsFolded: {
      l_arm: { x: -0.25, y: 0.28, z: -0.78 },
      r_arm: { x: 0, y: -0.08, z: 1.38 },
      l_fore: { x: 0, y: 1.05, z: 0 },
      r_fore: { x: 0, y: -0.18, z: 0 },
      spine: { x: 0, y: -0.04, z: 0 },
      neck: { x: 0.16, y: -0.06, z: 0.06 }
    },
    // 姿态3：专注倾听（双臂在腹前轻轻交错，脑袋稍稍歪斜思索）
    pensive: {
      l_arm: { x: -0.22, y: 0.42, z: -0.62 },
      r_arm: { x: -0.32, y: -0.32, z: 0.62 },
      l_fore: { x: 0, y: 0.85, z: 0 },
      r_fore: { x: 0, y: -0.85, z: 0 },
      spine: { x: -0.02, y: 0.04, z: 0 },
      neck: { x: 0.15, y: 0.10, z: 0.04 }
    }
  };

  let currentIdlePose = 'handsBehind'; // 一开始显示就是交叠背在腰后，符合用户特定要求！
  let lastPoseSwitchTime = 0;
  let nextPoseSwitchInterval = 8; // 首度切换定在 8 秒后

  function animate() {
    threeAnimId = requestAnimationFrame(animate);

    const delta = clock.getDelta();
    const time = clock.getElapsedTime();

    // ---------------------------------------------------------------------------
    // 🎨 骨骼与表情驱动核心 (Skeletal & Expression Drive Core) - 高效缓存驱动，完美消灭呆板
    // ---------------------------------------------------------------------------
    if (characterModel) {
      // 计算实时声能音量 (用于驱动骨骼微物理动作)
      let activeVolume = 0;
      if (isRecording.value) {
        if (analyser) {
          const dataArray = new Uint8Array(analyser.frequencyBinCount);
          analyser.getByteFrequencyData(dataArray);
          let sum = 0;
          for (let i = 0; i < dataArray.length; i++) {
            sum += dataArray[i];
          }
          activeVolume = sum / dataArray.length / 255.0;
        }
      } else if (isSpeaking.value) {
        // 模拟高保真发音音轨振幅
        activeVolume = 0.15 + Math.abs(Math.sin(time * 14.0)) * 0.45 + Math.cos(time * 5.0) * 0.1;
        activeVolume = Math.max(0, Math.min(1, activeVolume));
      }

      // 待机姿态定时轮替
      if (!isRecording.value && !isSpeaking.value) {
        if (time - lastPoseSwitchTime > nextPoseSwitchInterval) {
          const poseKeys = Object.keys(idlePoses);
          let nextKey = currentIdlePose;
          while (nextKey === currentIdlePose) {
            nextKey = poseKeys[Math.floor(Math.random() * poseKeys.length)];
          }
          currentIdlePose = nextKey;
          lastPoseSwitchTime = time;
          nextPoseSwitchInterval = 10 + Math.random() * 8; // 10 到 18 秒之间随机轮转
        }
      }

      // --- 1. 左大臂 (Left Upper Arm) ---
      if (characterBones.leftUpperArm) {
        const bone = characterBones.leftUpperArm;
        if (isRecording.value) {
          // 聆听姿态：双手对称微拱前倾
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, -0.55 + Math.sin(time * 2.0) * 0.05 * activeVolume, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, 0.25, 0.08);
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, -0.3 + Math.sin(time * 2.2) * 0.04, 0.08);
        } else if (isSpeaking.value) {
          // 说话姿态：左手臂随身体有节奏晃动
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, -1.05 + Math.sin(time * 3.0) * 0.08 * activeVolume, 0.08);
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, -0.35 + Math.cos(time * 2.0) * 0.06, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, 0.15, 0.08);
        } else {
          // 待机动作：加上胸腹微弱的呼吸慢正弦曲线起伏
          const target = idlePoses[currentIdlePose].l_arm;
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, target.x + Math.cos(time * 0.7) * 0.025, 0.03);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, target.y, 0.03);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, target.z + Math.sin(time * 0.9) * 0.025, 0.03);
        }
      }

      // --- 2. 右大臂 (Right Upper Arm) ---
      if (characterBones.rightUpperArm) {
        const bone = characterBones.rightUpperArm;
        if (isRecording.value) {
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, 0.55 - Math.sin(time * 2.0) * 0.05 * activeVolume, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, -0.25, 0.08);
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, -0.3 + Math.sin(time * 2.2) * 0.04, 0.08);
        } else if (isSpeaking.value) {
          // 说话姿态：高举起右手，手肘和手腕随发音大小高频挥舞解释，栩栩如生！
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, 0.55 + Math.sin(time * 8.5) * 0.35 * activeVolume, 0.08);
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, -0.98 + Math.cos(time * 5.5) * 0.18 * activeVolume, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, -0.25, 0.08);
        } else {
          const target = idlePoses[currentIdlePose].r_arm;
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, target.x + Math.cos(time * 0.7) * 0.025, 0.03);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, target.y, 0.03);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, target.z - Math.sin(time * 0.9) * 0.025, 0.03);
        }
      }

      // --- 3. 左小臂 (Left Forearm) ---
      if (characterBones.leftForearm) {
        const bone = characterBones.leftForearm;
        if (isRecording.value) {
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, 0.95, 0.08);
        } else if (isSpeaking.value) {
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, 0.45 + Math.sin(time * 3.5) * 0.08 * activeVolume, 0.08);
        } else {
          const target = idlePoses[currentIdlePose].l_fore;
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, target.x, 0.03);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, target.y + Math.sin(time * 0.8) * 0.015, 0.03);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, target.z, 0.03);
        }
      }

      // --- 4. 右小臂 (Right Forearm) ---
      if (characterBones.rightForearm) {
        const bone = characterBones.rightForearm;
        if (isRecording.value) {
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, -0.95, 0.08);
        } else if (isSpeaking.value) {
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, -0.8 + Math.cos(time * 8.5) * 0.2 * activeVolume, 0.08);
        } else {
          const target = idlePoses[currentIdlePose].r_fore;
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, target.x, 0.03);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, target.y - Math.sin(time * 0.8) * 0.015, 0.03);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, target.z, 0.03);
        }
      }

      // --- 5. 脊椎与躯干 (Spine / Chest) ---
      if (characterBones.spine) {
        const bone = characterBones.spine;
        if (isRecording.value) {
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, 0.02 + Math.sin(time * 2.2) * 0.015 * activeVolume, 0.08);
        } else if (isSpeaking.value) {
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, Math.sin(time * 4.0) * 0.035 * activeVolume, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, Math.cos(time * 3.0) * 0.025 * activeVolume, 0.08);
        } else {
          const target = idlePoses[currentIdlePose].spine;
          // 加呼吸微动
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, target.x + Math.sin(time * 1.2) * 0.015, 0.03);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, target.y, 0.03);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, target.z + Math.cos(time * 0.8) * 0.01, 0.03);
        }
      }

      // --- 6. 脖子与头部 (Neck & Head) ---
      if (characterBones.neck) {
        const bone = characterBones.neck;
        if (isRecording.value) {
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, 0.08 + Math.sin(time * 1.5) * 0.02, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, Math.cos(time * 1.2) * 0.035 * activeVolume, 0.08);
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, 0.12, 0.08); // 保持直视前倾角度
        } else if (isSpeaking.value) {
          // 点头晃脑歪头，极其灵动！
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, 0.14 + Math.sin(time * 6.0) * 0.06 * activeVolume, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, Math.cos(time * 4.0) * 0.09 * activeVolume, 0.08);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, -0.04 + Math.sin(time * 5.0) * 0.05 * activeVolume, 0.08);
        } else {
          const target = idlePoses[currentIdlePose].neck;
          // 待机点头微小晃动，保持平视前倾
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, target.x + Math.sin(time * 0.5) * 0.02, 0.03);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, target.y + Math.cos(time * 0.4) * 0.025, 0.03);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, target.z + Math.sin(time * 0.6) * 0.018, 0.03);
        }
      }

      // 让头部也保持完美的直视目光
      if (characterBones.head) {
        const bone = characterBones.head;
        if (isRecording.value) {
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, 0.04, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, 0, 0.08);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, 0, 0.08);
        } else if (isSpeaking.value) {
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, 0.04 + Math.cos(time * 5.0) * 0.03 * activeVolume, 0.08);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, Math.sin(time * 3.0) * 0.04 * activeVolume, 0.08);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, 0, 0.08);
        } else {
          // 待机时保持平视
          bone.rotation.x = THREE.MathUtils.lerp(bone.rotation.x, 0.04, 0.03);
          bone.rotation.y = THREE.MathUtils.lerp(bone.rotation.y, 0, 0.03);
          bone.rotation.z = THREE.MathUtils.lerp(bone.rotation.z, 0, 0.03);
        }
      }

      // --- 7. 表情与眨眼，发音嘴型驱动 (Face Morph Targets) ---
      if (faceMorphs.mesh && faceMorphs.mesh.morphTargetInfluences) {
        const influences = faceMorphs.mesh.morphTargetInfluences;
        
        // 自然眨眼控制
        if (faceMorphs.blinkIndex !== -1) {
          if (blinkProgress === -1) {
            if (time - lastBlinkTime > nextBlinkInterval) {
              blinkProgress = 0;
              lastBlinkTime = time;
              nextBlinkInterval = 2.5 + Math.random() * 3.0; // 2.5到5.5秒眨一次
            }
          } else {
            blinkProgress += delta / blinkDuration;
            if (blinkProgress >= 1.0) {
              blinkProgress = -1;
              influences[faceMorphs.blinkIndex] = 0;
            } else {
              influences[faceMorphs.blinkIndex] = Math.sin(blinkProgress * Math.PI);
            }
          }
        }
        
        // 语音驱动张嘴开合 (音视口型同步)
        if (faceMorphs.mouthIndex !== -1) {
          if (isSpeaking.value) {
            const targetMouthValue = Math.min(0.9, activeVolume * 1.5);
            influences[faceMorphs.mouthIndex] = THREE.MathUtils.lerp(
              influences[faceMorphs.mouthIndex],
              targetMouthValue,
              0.15
            );
          } else {
            // 平滑闭合
            influences[faceMorphs.mouthIndex] = THREE.MathUtils.lerp(
              influences[faceMorphs.mouthIndex],
              0,
              0.12
            );
          }
        }
      }
    }

    // ---------------------------------------------------------------------------
    // 鼠标拖拽平滑缓冲旋转控制 (LERP) - 默认正面面对屏幕 (targetRotationY = 0)
    // ---------------------------------------------------------------------------
    currentRotationY += (targetRotationY - currentRotationY) * 0.1; // 平滑缓冲
    hologramGroup.rotation.y = currentRotationY; // 完全消除自动旋转/摆动，死死锁定默认正面面对屏幕

    // 整个全息体悬浮升降漂移
    hologramGroup.position.y = Math.sin(time * 1.0) * 0.05;

    // 旋转科技环
    rings.forEach(r => {
      r.mesh.rotation.x += r.speedX;
      r.mesh.rotation.y += r.speedY;
    });

    // 激光线往复扫描
    scanLine.position.y += scanDir * 0.012;
    if (scanLine.position.y > 1.3) scanDir = -1;
    if (scanLine.position.y < -2.1) scanDir = 1;
    scanLine.rotation.y += 0.008;

    // 更新飘浮数字雨
    const posArr = particleSystem.geometry.attributes.position.array;
    for (let i = 0; i < particleCount; i++) {
      const vel = velocities[i];
      posArr[i * 3 + 1] += vel.y;
      
      vel.angle += vel.angleSpeed;
      posArr[i * 3] = Math.cos(vel.angle) * vel.radius;
      posArr[i * 3 + 2] = Math.sin(vel.angle) * vel.radius;

      if (posArr[i * 3 + 1] > 3.2) {
        posArr[i * 3 + 1] = -3.2;
      }
    }
    particleSystem.geometry.attributes.position.needsUpdate = true;

    // 全息粒子心跳表情波动
    let targetCoreScale = 1.0;
    let flickerChance = 0.02;

    if (isRecording.value) {
      let audioVolume = 0;
      if (analyser) {
        const dataArray = new Uint8Array(analyser.frequencyBinCount);
        analyser.getByteFrequencyData(dataArray);
        let sum = 0;
        for (let i = 0; i < dataArray.length; i++) {
          sum += dataArray[i];
        }
        audioVolume = sum / dataArray.length / 255.0;
      }

      targetCoreScale = 1.0 + audioVolume * 0.7;
      flickerChance = 0.03 + audioVolume * 0.12;

      applyCharacterMeshStyling(audioVolume);
    } else if (isSpeaking.value) {
      targetCoreScale = 1.0 + Math.sin(time * 16.0) * 0.1;
      flickerChance = 0.06;

      const speakingVolume = Math.abs(Math.sin(time * 12.0)) * 0.08;
      applyCharacterMeshStyling(speakingVolume);

      if (Math.random() < 0.05 && soundWaves.length < 5) {
        const waveGeom = new THREE.RingGeometry(0.7, 0.75, 32);
        const waveMesh = new THREE.Mesh(waveGeom, waveMat.clone());
        waveMesh.position.copy(hologramGroup.position);
        waveMesh.position.y += 0.3;
        waveMesh.rotation.x = Math.PI / 2;
        threeScene.add(waveMesh);
        soundWaves.push({ mesh: waveMesh, scale: 1.0, maxScale: 4.5 + Math.random() * 2.0, opacity: 0.9 });
      }
    } else {
      targetCoreScale = 0.95 + Math.sin(time * 0.6) * 0.05;
      flickerChance = 0.005;
      
      applyCharacterMeshStyling(0);
    }

    coreMesh.scale.lerp(new THREE.Vector3(targetCoreScale, targetCoreScale, targetCoreScale), 0.15);

    if (Math.random() < flickerChance) {
      coreMesh.visible = false;
      if (characterModel) {
        characterModel.traverse((child) => {
          if (child.isMesh && child.material) child.material.opacity = 0.05;
        });
      }
      particleMat.opacity = 0.1;
      scanMat.opacity = 0.02;
    } else {
      coreMesh.visible = true;
      // 粒子和激光线透明度
      particleMat.opacity = isActive.value ? 0.75 : 0.35;
      scanMat.opacity = isActive.value ? 0.45 : 0.15;
    }

    for (let i = soundWaves.length - 1; i >= 0; i--) {
      const wave = soundWaves[i];
      wave.scale += 0.085;
      wave.opacity -= 0.015;
      wave.mesh.scale.set(wave.scale, wave.scale, 1.0);
      wave.mesh.material.opacity = wave.opacity;

      if (wave.scale > wave.maxScale || wave.opacity <= 0) {
        threeScene.remove(wave.mesh);
        wave.mesh.geometry.dispose();
        wave.mesh.material.dispose();
        soundWaves.splice(i, 1);
      }
    }

    threeRenderer.render(threeScene, threeCamera);
  }

  animate();
}

function handleThreeResize() {
  const canvas = threeCanvasRef.value;
  if (!canvas || !threeRenderer || !threeCamera) return;

  const width = canvas.clientWidth;
  const height = canvas.clientHeight;

  threeCamera.aspect = width / height;
  threeCamera.updateProjectionMatrix();

  threeRenderer.setSize(width, height);
}

// ---------------------------------------------------------------------------
// 控制方法与事件处理
// ---------------------------------------------------------------------------
async function startVoice() {
  try {
    await connectWebSocket();
    await startMicrophone();
    ElMessage.success('安全语音信道激活成功');
  } catch (err) {
    if (ws) ws.close();
  }
}

function toggleRecording() {
  if (isRecording.value) {
    isRecording.value = false;
    addMessage('system', { message: '麦克风已下线' });
  } else {
    isRecording.value = true;
    addMessage('system', { message: '麦克风恢复工作' });
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send("interrupt");
    }
    if (typeof window !== 'undefined' && window.speechSynthesis) {
      window.speechSynthesis.cancel();
    }
  }
}

function stopVoice() {
  stopMicrophone();
  if (ws) {
    ws.close();
    ws = null;
  }
  isConnected.value = false;
  isActive.value = false;
  currentText.value = '';
}

// ---------------------------------------------------------------------------
// 消息管理与列表自动滚动
// ---------------------------------------------------------------------------
function addMessage(type, msg) {
  const now = new Date();
  const time = now.toLocaleTimeString('zh-CN', { hour12: false });

  messages.value.push({
    type,
    text: msg.text || '',
    message: msg.message || '',
    intent: msg.intent || '',
    status: msg.status || '',
    time,
  });

  nextTick(() => {
    const list = messageListRef.value;
    if (list) {
      list.scrollTop = list.scrollHeight;
    }
  });
}

// ---------------------------------------------------------------------------
// 挂载与卸载清理
// ---------------------------------------------------------------------------
onMounted(() => {
  loadVoices();
  if (typeof window !== 'undefined' && window.speechSynthesis) {
    window.speechSynthesis.onvoiceschanged = loadVoices;
  }

  // 初始化 Three.js 红皇后投影与骨骼载入
  initThreeHologram();
  window.addEventListener('resize', handleThreeResize);

  // 绑定鼠标/触控拖拽左右滑动事件，实现自由视角旋转
  window.addEventListener('mousedown', handleMouseDown);
  window.addEventListener('mousemove', handleMouseMove);
  window.addEventListener('mouseup', handleMouseUp);
  
  window.addEventListener('touchstart', handleTouchStart, { passive: true });
  window.addEventListener('touchmove', handleTouchMove, { passive: true });
  window.addEventListener('touchend', handleMouseUp);

  // HUD CPU 负载波动模拟
  hudInterval = setInterval(() => {
    cpuLoad.value = Math.floor(18 + Math.random() * 15);
  }, 3000);
});

onUnmounted(() => {
  stopVoice();
  
  if (threeAnimId) {
    cancelAnimationFrame(threeAnimId);
  }
  if (threeRenderer) {
    threeRenderer.dispose();
  }
  window.removeEventListener('resize', handleThreeResize);
  
  // 卸载鼠标/触控拖拽事件
  window.removeEventListener('mousedown', handleMouseDown);
  window.removeEventListener('mousemove', handleMouseMove);
  window.removeEventListener('mouseup', handleMouseUp);
  
  window.removeEventListener('touchstart', handleTouchStart);
  window.removeEventListener('touchmove', handleTouchMove);
  window.removeEventListener('touchend', handleMouseUp);

  if (hudInterval) {
    clearInterval(hudInterval);
  }
});
</script>

<style scoped>
/* ---------------------------------------------------------------------------
   页面全屏布局
   --------------------------------------------------------------------------- */
.voice-terminal-container {
  position: relative;
  width: 100%;
  height: calc(100vh - 100px); /* 适应后台顶栏/边栏高度 */
  background: #090005;
  border-radius: 16px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* 3D全息画布容器 */
.hologram-canvas-container {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  z-index: 1;
}

.hologram-canvas {
  width: 100%;
  height: 100%;
  display: block;
  cursor: grab; /* 拖拽悬浮提示 */
}
.hologram-canvas:active {
  cursor: grabbing;
}

/* ---------------------------------------------------------------------------
   生化危机 HUD 科幻效果
   --------------------------------------------------------------------------- */
.hud-overlay {
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: 2;
  border: 1px solid rgba(255, 0, 85, 0.08);
}

.hud-corner {
  position: absolute;
  width: 16px;
  height: 16px;
  border-color: #ff0055;
  border-style: solid;
  opacity: 0.65;
}
.top-left { top: 12px; left: 12px; border-width: 2px 0 0 2px; }
.top-right { top: 12px; right: 12px; border-width: 2px 2px 0 0; }
.bottom-left { bottom: 12px; left: 12px; border-width: 0 0 2px 2px; }
.bottom-right { bottom: 12px; right: 12px; border-width: 0 2px 2px 0; }

.hud-grid-line {
  position: absolute;
  top: 15%;
  left: 0;
  width: 100%;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(255,0,85,0.12), transparent);
}

.hud-system-info {
  position: absolute;
  top: 16px;
  left: 20px;
  display: flex;
  align-items: center;
  gap: 12px;
  color: #ff0055;
  font-family: 'Courier New', Courier, monospace;
}

.hud-logo-icon {
  font-size: 20px;
  color: #ff0055;
  text-shadow: 0 0 8px #ff0055;
  animation: logo-blink 2s infinite;
}

.model-status-tag {
  font-size: 9px;
  background: rgba(255, 0, 85, 0.15);
  border: 1px solid rgba(255, 0, 85, 0.3);
  padding: 1px 4px;
  border-radius: 2px;
  margin-top: 4px;
  display: inline-block;
  color: #ff3366;
  font-weight: bold;
}
.model-loaded {
  background: rgba(103, 194, 58, 0.15);
  color: #67c23a;
  border-color: rgba(103, 194, 58, 0.4);
}

@keyframes logo-blink {
  0%, 100% { opacity: 0.8; }
  50% { opacity: 0.3; }
}

.system-status-text {
  font-size: 11px;
  line-height: 1.4;
  opacity: 0.75;
  letter-spacing: 0.5px;
}

.hud-scanners {
  position: absolute;
  top: 16px;
  right: 20px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  color: #ff0055;
  font-family: 'Courier New', Courier, monospace;
  font-size: 10px;
}

.hud-scanner-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  background: rgba(255, 0, 85, 0.08);
  padding: 2px 6px;
  border-radius: 2px;
  border: 1px solid rgba(255, 0, 85, 0.15);
}

.scanner-label {
  opacity: 0.6;
}
.scanner-val {
  font-weight: bold;
}

/* ---------------------------------------------------------------------------
   悬浮玻璃拟态 UI 布局
   --------------------------------------------------------------------------- */
.terminal-ui-overlay {
  position: absolute;
  inset: 0;
  z-index: 3;
  padding: 24px;
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  pointer-events: none;
}

.interactive-left-panel,
.interactive-right-panel {
  pointer-events: auto;
  width: 48%;
  max-width: 480px;
  display: flex;
  flex-direction: column;
}

/* 玻璃拟态基础卡片 */
.glass-panel {
  background: rgba(12, 5, 8, 0.62);
  backdrop-filter: blur(15px) saturate(180%);
  -webkit-backdrop-filter: blur(15px) saturate(180%);
  border: 1px solid rgba(255, 0, 85, 0.18);
  border-radius: 14px;
  box-shadow: 0 12px 30px rgba(0, 0, 0, 0.6), 0 0 15px rgba(255, 0, 85, 0.05);
  padding: 20px;
  color: #fff;
  transition: all 0.4s ease;
}

.voice-control-card {
  border-left: 4px solid #ff0055;
}

.voice-control-card.active {
  box-shadow: 0 12px 35px rgba(0, 0, 0, 0.7), 0 0 25px rgba(255, 0, 85, 0.25);
  border-color: #ff3366;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  border-bottom: 1px solid rgba(255, 0, 85, 0.15);
  padding-bottom: 8px;
}

.panel-title {
  font-weight: 700;
  font-size: 14px;
  color: #ff3366;
  text-shadow: 0 0 6px rgba(255, 0, 85, 0.5);
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 状态指示点 */
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}
.dot-active {
  background: #ff0055;
  box-shadow: 0 0 10px #ff0055;
  animation: pulse-red-dot 1.2s infinite;
}
.dot-sleeping {
  background: #aa0033;
  box-shadow: 0 0 4px #aa0033;
}
.dot-disconnected {
  background: #555;
}

@keyframes pulse-red-dot {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.3); opacity: 0.6; }
}

/* 正在聆听标签 */
.pulse-recording-tag {
  display: flex;
  align-items: center;
  gap: 6px;
}
.pulse-ring-red {
  width: 8px;
  height: 8px;
  background: #ff3366;
  border-radius: 50%;
  animation: pulse-rec 1s infinite;
}
.pulse-text-red {
  color: #ff3366;
  font-size: 12px;
  font-weight: 600;
}

/* 实时识别文本框 */
.recognition-glass-area {
  background: rgba(255, 0, 85, 0.05);
  border: 1px solid rgba(255, 0, 85, 0.12);
  border-radius: 8px;
  padding: 12px;
  min-height: 70px;
  margin-bottom: 16px;
}

.recognition-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  margin-bottom: 4px;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.recognition-content {
  font-size: 15px;
  color: rgba(255, 255, 255, 0.95);
  font-weight: 500;
  line-height: 1.5;
}

.recognition-content.content-partial {
  color: rgba(255, 255, 255, 0.5);
  font-style: italic;
}

/* 2D波形频谱 */
.mini-waveform-container {
  height: 40px;
  margin-bottom: 16px;
}
.waveform-canvas-mini {
  width: 100%;
  height: 100%;
  display: block;
}

/* 交互按钮 */
.control-buttons-group {
  display: flex;
  gap: 12px;
}

.cyber-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: rgba(255, 0, 85, 0.18);
  border: 1px solid rgba(255, 0, 85, 0.4);
  color: #fff;
  border-radius: 6px;
  height: 40px;
  font-weight: 600;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.3s;
  text-shadow: 0 0 4px rgba(0,0,0,0.5);
}

.cyber-btn:hover {
  background: rgba(255, 0, 85, 0.35);
  box-shadow: 0 0 10px rgba(255, 0, 85, 0.3);
  transform: translateY(-1px);
}

.cyber-btn.btn-primary {
  background: rgba(255, 0, 85, 0.35);
  border-color: #ff0055;
  box-shadow: 0 0 12px rgba(255, 0, 85, 0.2);
}
.cyber-btn.btn-primary:hover {
  background: rgba(255, 0, 85, 0.55);
  box-shadow: 0 0 18px rgba(255, 0, 85, 0.45);
}

.cyber-btn.btn-danger {
  background: rgba(245, 108, 108, 0.2);
  border-color: #f56c6c;
}
.cyber-btn.btn-danger:hover {
  background: rgba(245, 108, 108, 0.4);
  box-shadow: 0 0 12px rgba(245, 108, 108, 0.3);
}

.cyber-btn.btn-success {
  background: rgba(103, 194, 58, 0.2);
  border-color: #67c23a;
}
.cyber-btn.btn-success:hover {
  background: rgba(103, 194, 58, 0.4);
  box-shadow: 0 0 12px rgba(103, 194, 58, 0.3);
}

.cyber-btn.btn-secondary {
  flex: 0.6;
  background: rgba(255,255,255,0.05);
  border-color: rgba(255,255,255,0.15);
}
.cyber-btn.btn-secondary:hover {
  background: rgba(255,255,255,0.15);
  border-color: rgba(255,255,255,0.3);
}

/* ---------------------------------------------------------------------------
   右侧日志时间线
   --------------------------------------------------------------------------- */
.message-history-card {
  height: 380px;
  display: flex;
  flex-direction: column;
}

.history-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cyber-select :deep(.el-input__wrapper) {
  background-color: rgba(0, 0, 0, 0.45) !important;
  border: 1px solid rgba(255, 0, 85, 0.25) !important;
  box-shadow: none !important;
}
.cyber-select :deep(.el-input__inner) {
  color: #fff !important;
  font-size: 11px;
}

.cyber-btn-mini {
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.15);
  color: rgba(255,255,255,0.7);
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
  transition: all 0.2s;
}
.cyber-btn-mini:hover {
  background: rgba(255, 0, 85, 0.2);
  color: #fff;
  border-color: rgba(255,0,85,0.3);
}

.message-list {
  flex: 1;
  overflow-y: auto;
  padding-right: 4px;
  margin-top: 10px;
}

.empty-messages {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: rgba(255,255,255,0.3);
  font-size: 12px;
  gap: 8px;
}

.no-logs-icon {
  font-size: 24px;
  color: rgba(255,0,85,0.15);
  animation: logo-blink 2s infinite;
}

.message-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 8px 12px;
  border-radius: 8px;
  margin-bottom: 8px;
  border-left: 2px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.02);
  transition: background 0.2s;
}
.message-item:hover {
  background: rgba(255, 0, 85, 0.04);
}

.msg-icon-glow {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: rgba(255,255,255,0.3);
  margin-top: 6px;
  flex-shrink: 0;
}

.msg-body {
  flex: 1;
  min-width: 0;
}
.msg-content {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.95);
  word-break: break-all;
  line-height: 1.4;
}
.msg-text {
  font-weight: bold;
  color: #ff3366;
  margin-right: 6px;
}
.msg-desc {
  color: rgba(255,255,255,0.75);
}
.msg-time {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.3);
  margin-top: 4px;
}

.msg-intent-tag {
  display: inline-block;
  font-size: 10px;
  padding: 0 5px;
  border-radius: 3px;
  font-weight: bold;
  margin-left: 8px;
}
.tag-success {
  background: rgba(103, 194, 58, 0.15);
  color: #67c23a;
  border: 1px solid rgba(103,194,58,0.25);
}
.tag-failed {
  background: rgba(245, 108, 108, 0.15);
  color: #f56c6c;
  border: 1px solid rgba(245,108,108,0.25);
}

/* 消息日志特定类型配色 */
.msg-wake {
  border-left-color: #ff3366;
  background: rgba(255, 0, 85, 0.05);
}
.msg-wake .msg-icon-glow {
  background: #ff0055;
  box-shadow: 0 0 6px #ff0055;
}

.msg-sleep {
  border-left-color: #aa0033;
  background: rgba(0, 0, 0, 0.2);
}
.msg-sleep .msg-icon-glow {
  background: #aa0033;
}

.msg-result {
  border-left-color: #ff0055;
  background: rgba(255, 0, 85, 0.08);
}
.msg-result .msg-icon-glow {
  background: #ff3366;
  box-shadow: 0 0 8px #ff3366;
}

.msg-system {
  opacity: 0.65;
}
.msg-system .msg-icon-glow {
  background: #555;
}

.msg-final {
  border-left-color: rgba(255,255,255,0.4);
}
.msg-final .msg-icon-glow {
  background: rgba(255,255,255,0.5);
}

/* ---------------------------------------------------------------------------
   通用滚动条与动画
   --------------------------------------------------------------------------- */
.message-list::-webkit-scrollbar {
  width: 4px;
}
.message-list::-webkit-scrollbar-track {
  background: transparent;
}
.message-list::-webkit-scrollbar-thumb {
  background: rgba(255, 0, 85, 0.2);
  border-radius: 2px;
}
.message-list::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 0, 85, 0.4);
}

.msg-enter-active {
  transition: all 0.35s cubic-bezier(0.16, 1, 0.3, 1);
}
.msg-enter-from {
  opacity: 0;
  transform: translateY(12px) scale(0.98);
}

/* ---------------------------------------------------------------------------
   全息设置控制板
   --------------------------------------------------------------------------- */
.hologram-settings-card {
  margin-top: 14px;
  border-left: 4px solid #ff3366;
}

.hologram-settings-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.setting-label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.7);
  font-weight: 600;
  text-shadow: 0 0 2px rgba(0,0,0,0.5);
}

.cyber-radio-group :deep(.el-radio-button__inner) {
  background-color: rgba(0, 0, 0, 0.45) !important;
  border-color: rgba(255, 0, 85, 0.25) !important;
  color: rgba(255, 255, 255, 0.7) !important;
  font-size: 11px;
}
.cyber-radio-group :deep(.el-radio-button__orig-radio:checked + .el-radio-button__inner) {
  background-color: rgba(255, 0, 85, 0.35) !important;
  border-color: #ff0055 !important;
  color: #fff !important;
  box-shadow: -1px 0 0 0 #ff0055 !important;
}

.custom-model-importer {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 4px;
}

.drag-drop-zone {
  border: 1px dashed rgba(255, 0, 85, 0.4);
  background: rgba(255, 0, 85, 0.04);
  border-radius: 8px;
  padding: 14px;
  text-align: center;
  cursor: pointer;
  transition: all 0.3s ease;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.drag-drop-zone:hover, .drag-drop-zone.dragging {
  border-color: #ff3366;
  background: rgba(255, 0, 85, 0.08);
  box-shadow: 0 0 10px rgba(255, 0, 85, 0.15);
}

.upload-icon {
  font-size: 24px;
  color: #ff3366;
  filter: drop-shadow(0 0 4px rgba(255,0,85,0.4));
}

.upload-text {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.85);
  line-height: 1.4;
}

.upload-text b {
  color: #ff3366;
}

.importer-actions {
  display: flex;
  justify-content: flex-end;
}

.btn-danger-mini {
  background: rgba(245, 108, 108, 0.2);
  border: 1px solid #f56c6c;
  color: #f56c6c;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-danger-mini:hover {
  background: rgba(245, 108, 108, 0.4);
  color: #fff;
}

.importer-tips {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
  line-height: 1.4;
  border-top: 1px dashed rgba(255, 255, 255, 0.08);
  padding-top: 6px;
}
</style>
