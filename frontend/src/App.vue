<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'

const screens = ref([])
const busy = ref('')
const message = ref('')
const failed = ref(false)
const highlighted = ref('')
const connectedCount = computed(() => screens.value.filter(screen => screen.connected).length)
const api = () => window.go?.display?.Service

async function perform(action, work) {
  if (busy.value) return
  busy.value = action
  message.value = ''
  failed.value = false
  try {
    if (!api()) throw new Error('请在屏幕管理工具的桌面窗口中使用。')
    await work()
  } catch (error) {
    message.value = error?.message || String(error)
    failed.value = true
  } finally {
    busy.value = ''
  }
}

async function refresh() {
  screens.value = await api().ListDisplays()
}

function identify() {
  perform('identify', async () => {
    await api().IdentifyDisplays(screens.value.map(screen => screen.id))
  })
}

function power(screen) {
  perform(screen.id, async () => {
    const on = screen.power === 'off'
    if (!on && !screens.value.some(other => other.id !== screen.id && other.connected && other.power !== 'off')) {
      if (!window.confirm('这是当前唯一可用的屏幕。关闭后可能需要按显示器上的电源键恢复。继续关闭吗？')) return
    }
    const feedback = await api().SetDisplayPower(screen.id, on)
    await new Promise(resolve => setTimeout(resolve, 500))
    await refresh()
    const current = screens.value.find(item => item.id === screen.id)
    message.value = current?.power === (on ? 'on' : 'off')
      ? (on ? '显示器已开启。' : '显示器已关闭。')
      : feedback.replace('正在刷新显示器状态', '当前电源状态无法确认')
  })
}

function changeConnection(screen) {
  perform(screen.id, async () => {
    const connect = screen.disconnected
    await api().SetDisplayConnection(screen.id, connect)
    await refresh()
    const updated = screens.value.find(item => item.id === screen.id)
    if (!connect && updated?.connected) {
      throw new Error('系统仍把这台显示器列为已连接。')
    }
    message.value = connect ? '已提交重新连接，请查看显示器。' : '显示器已从桌面断开。'
  })
}

function displayState(screen) {
  if (screen.disconnected) return { label: '已断开', tone: 'offline' }
  if (!screen.connected) return { label: '未连接', tone: 'offline' }
  if (screen.power === 'off') return { label: '已关屏', tone: 'standby' }
  return { label: screen.power === 'on' ? '已点亮' : '已连接', tone: 'online' }
}

async function showDisplay(index) {
  await nextTick()
  document.getElementById(`display-${index}`)?.focus()
}
onMounted(() => perform('refresh', refresh))
</script>

<template>
  <main class="panel">
    <header class="app-header">
      <h1>显示器</h1>
      <div class="toolbar">
        <button :disabled="!!busy || !connectedCount" @click="identify">
          <span class="button-icon icon-identify" aria-hidden="true"></span>
          {{ busy === 'identify' ? '识别中…' : '识别屏幕' }}
        </button>
        <button :disabled="!!busy" @click="perform('refresh', refresh)">
          <span class="button-icon icon-refresh" :class="{ spinning: busy === 'refresh' }" aria-hidden="true">↻</span>
          {{ busy === 'refresh' ? '刷新中…' : '刷新' }}
        </button>
      </div>
    </header>

    <section class="overview" aria-labelledby="overview-title">
      <div class="overview-heading">
        <h2 id="overview-title">屏幕概览</h2>
        <p class="overview-count"><span class="status-dot" :class="{ online: connectedCount }"></span>{{ connectedCount }} 台已连接</p>
      </div>
      <TransitionGroup v-if="screens.length" name="monitor" tag="ul" appear class="monitor-grid" aria-label="选择显示器">
        <li v-for="(screen, index) in screens" :key="screen.id">
          <button
            class="monitor"
            :class="[displayState(screen).tone, { selected: highlighted === screen.id, processing: busy === screen.id }]"
            :aria-label="`查看显示器 ${index + 1}，${screen.name}，${displayState(screen).label}`"
            @click="showDisplay(index)"
          >
            <span class="monitor-frame" :class="{ 'built-in': screen.internal }" aria-hidden="true">
              <span class="monitor-panel">{{ String(index + 1).padStart(2, '0') }}</span>
            </span>
            <span class="monitor-label">{{ screen.name }}</span>
          </button>
        </li>
      </TransitionGroup>
      <p v-else class="empty">
        {{ busy === 'refresh' ? '正在查找显示器…' : failed ? '无法读取显示器' : '未检测到显示器' }}
      </p>
    </section>

    <section v-if="screens.length" class="device-section" aria-labelledby="devices-title">
      <h2 id="devices-title">显示器控制</h2>
      <TransitionGroup name="device" tag="ul" appear class="display-list">
        <li
          v-for="(screen, index) in screens"
          :id="`display-${index}`"
          :key="screen.id"
          class="display-row"
          :class="{ selected: highlighted === screen.id, processing: busy === screen.id }"
          :aria-busy="busy === screen.id"
          tabindex="-1"
          @focus="highlighted = screen.id"
          @blur="highlighted = ''"
        >
          <span class="device-number" aria-hidden="true">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="details">
            <div class="device-heading">
              <h3>{{ screen.name }}</h3>
              <span v-if="screen.primary" class="primary-label">主显示器</span>
            </div>
            <p class="specs">
              <span>{{ screen.internal ? '内置屏幕' : '外接显示器' }}</span>
              <span v-if="screen.width && screen.height">{{ screen.width }} × {{ screen.height }}</span>
            </p>
            <p v-if="screen.reason" class="reason">{{ screen.reason }}</p>
          </div>
          <div class="device-controls">
            <span class="status" :class="displayState(screen).tone">
              <span class="status-dot" aria-hidden="true"></span>{{ displayState(screen).label }}
            </span>
            <div v-if="screen.canPower || screen.canDisconnect || screen.disconnected" class="actions">
              <button
                v-if="screen.canPower && screen.power !== 'unknown'"
                class="subtle"
                :disabled="!!busy"
                @click="power(screen)"
              >{{ screen.power === 'on' ? '关屏' : '点亮' }}</button>
              <button
                v-if="screen.canDisconnect || screen.disconnected"
                :class="{ accent: screen.disconnected }"
                :disabled="!!busy"
                @click="changeConnection(screen)"
              >{{ screen.disconnected ? '重新连接' : '断开显示器' }}</button>
            </div>
          </div>
        </li>
      </TransitionGroup>
    </section>

    <div class="feedback-area" role="status" aria-live="polite">
      <Transition name="feedback">
        <p v-if="message" class="feedback-message" :class="{ failed }">
          <span class="status-dot" aria-hidden="true"></span>{{ message }}
        </p>
      </Transition>
    </div>
  </main>
</template>
