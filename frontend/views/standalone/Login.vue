<template>
  <div class="lg-page">
    <div class="lg-bg" aria-hidden="true">
      <i class="lg-blob lg-b1"></i>
      <i class="lg-blob lg-b2"></i>
      <i class="lg-blob lg-b3"></i>
      <i class="lg-grid"></i>
    </div>

    <section class="lg-stage">
      <div class="lg-brand">
        <svg class="lg-logo" viewBox="0 0 40 40" width="38" height="38" aria-hidden="true">
          <defs>
            <linearGradient id="lg-logo-g" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" stop-color="#22d3b8" />
              <stop offset="0.55" stop-color="#229ed9" />
              <stop offset="1" stop-color="#7c6cf0" />
            </linearGradient>
          </defs>
          <rect width="40" height="40" rx="12" fill="url(#lg-logo-g)" />
          <path d="M29 11 11 18.5l6 2.3 2.2 7 3-4.2 4.6 3.4z" fill="#04131c" />
          <path d="M17 20.8 29 11" stroke="#8ff3e2" stroke-width="1.4" fill="none" />
        </svg>
        <span>TG打卡考勤</span>
      </div>

      <div class="lg-copy">
        <h1>让每一次打卡，<br />都有清晰的记录。</h1>
        <p>Telegram 打卡、多班次、活动管理、审批与报表，集中在一个后台里。</p>
      </div>

      <div class="lg-dial-wrap">
        <div class="lg-dial" role="img" :aria-label="'当前时间 ' + hhmm + ':' + ss">
          <svg viewBox="0 0 400 400" aria-hidden="true">
            <defs>
              <linearGradient id="lg-tick-g" gradientUnits="userSpaceOnUse" x1="40" y1="60" x2="360" y2="340">
                <stop offset="0" stop-color="#22d3b8" />
                <stop offset="0.5" stop-color="#229ed9" />
                <stop offset="1" stop-color="#8b7cff" />
              </linearGradient>
              <radialGradient id="lg-core" cx="50%" cy="50%" r="50%">
                <stop offset="0" stop-color="#22d3b8" stop-opacity="0.16" />
                <stop offset="1" stop-color="#22d3b8" stop-opacity="0" />
              </radialGradient>
            </defs>
            <circle cx="200" cy="200" r="132" fill="url(#lg-core)" />
            <circle cx="200" cy="200" r="132" class="lg-ring-in" />
            <circle cx="200" cy="200" r="196" class="lg-ring-out" />
            <line
              v-for="t in ticks"
              :key="t.i"
              :x1="t.x1"
              :y1="t.y1"
              :x2="t.x2"
              :y2="t.y2"
              :class="['lg-tick', { major: t.major, lit: t.i <= sec, now: t.i === sec }]"
              :style="{ '--i': t.i }"
            />
            <g class="lg-orbit">
              <path d="M10 0 -8 -6.5 -3 0 -8 6.5z" transform="translate(200 4)" class="lg-plane" />
            </g>
          </svg>
          <div class="lg-readout">
            <div class="lg-time"><span>{{ hhmm }}</span><em>{{ ss }}</em></div>
            <div class="lg-date">{{ dateText }}</div>
          </div>
        </div>

        <div class="lg-chip lg-chip-a"><i></i>上班打卡已记录</div>
        <div class="lg-chip lg-chip-b"><i></i>活动与回座计时中</div>
        <div class="lg-chip lg-chip-c"><i></i>超时提醒自动推送</div>
      </div>

      <div class="lg-foot">独立部署，数据自持，保留原有打卡体验。</div>
    </section>

    <section class="lg-side">
      <div class="lg-card">
        <div class="lg-card-head">
          <svg class="lg-logo" viewBox="0 0 40 40" width="44" height="44" aria-hidden="true">
            <defs>
              <linearGradient id="lg-logo-g2" x1="0" y1="0" x2="1" y2="1">
                <stop offset="0" stop-color="#22d3b8" />
                <stop offset="0.55" stop-color="#229ed9" />
                <stop offset="1" stop-color="#7c6cf0" />
              </linearGradient>
            </defs>
            <rect width="40" height="40" rx="12" fill="url(#lg-logo-g2)" />
            <path d="M29 11 11 18.5l6 2.3 2.2 7 3-4.2 4.6 3.4z" fill="#04131c" />
            <path d="M17 20.8 29 11" stroke="#8ff3e2" stroke-width="1.4" fill="none" />
          </svg>
          <div>
            <h2>登录考勤工作台</h2>
            <p class="lg-muted">使用管理员账号或分配给你的子账号。</p>
          </div>
        </div>
        <el-form @submit.prevent="submit" class="login-fields lg-fields">
          <template v-if="!challenge">
            <el-form-item>
              <el-input
                v-model="username"
                :prefix-icon="User"
                autocomplete="username"
                placeholder="账号"
                size="large"
                @blur="loadMode"
              />
            </el-form-item>
            <el-form-item>
              <el-input
                v-model="password"
                :prefix-icon="Lock"
                type="password"
                show-password
                autocomplete="current-password"
                placeholder="密码"
                size="large"
              />
            </el-form-item>
            <el-form-item>
              <div class="verification-row">
                <el-input
                  v-model="code"
                  :prefix-icon="Key"
                  :maxlength="6"
                  autocomplete="one-time-code"
                  :placeholder="isTotp ? '谷歌验证码' : '验证码'"
                  size="large"
                />
                <template v-if="!isTotp">
                  <img
                    v-if="captcha"
                    :src="captcha.captchaImage"
                    alt="图片验证码，点击刷新"
                    title="点击刷新"
                    width="144"
                    height="52"
                    style="cursor: pointer"
                    @click="loadCaptcha"
                  /><el-button v-else link @click="loadCaptcha">加载验证码</el-button>
                </template>
              </div>
            </el-form-item>
            <el-checkbox v-model="remember" class="remember-box">记住账号密码</el-checkbox>
          </template>
          <template v-if="challenge">
            <div class="totp-setup"><p>首次绑定：在 Google Authenticator 中扫描二维码，或手动输入密钥，然后输入6位动态码完成绑定。</p><img :src="challenge.qrImage" alt="谷歌验证器绑定二维码" width="220" height="220"/><el-input :model-value="challenge.secret" readonly/><p class="lg-muted">请妥善保存密钥，绑定成功后不会再次显示。</p></div>
            <el-form-item label="谷歌动态验证码（6位数字）"><el-input v-model="code" :maxlength="6" autocomplete="one-time-code" size="large"/></el-form-item>
            <el-button link :disabled="busy" @click="restart">返回重新登录</el-button>
          </template>
          <el-alert
            v-if="error"
            :title="error"
            type="error"
            :closable="false"
          /><el-button
            native-type="submit"
            type="primary"
            size="large"
            class="lg-submit"
            :loading="busy"
            >{{challenge ? '验证并登录' : '登录'}}</el-button
          ></el-form
        >
        <p class="lg-muted lg-note">
          免费服务休眠后，首次连接可能需要约一分钟。
        </p>
      </div>
    </section>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { Key, Lock, User } from "@element-plus/icons-vue";
import { useRouter } from "vue-router";
import request from "../../api/request";
import { useUserStore } from "../../store/user";
const username = ref(""),
  password = ref(""),
  error = ref(""),
  busy = ref(false),
  router = useRouter(),
  user = useUserStore();
const challenge = ref<any>(null),
  captcha = ref<any>(null),
  code = ref(""),
  remember = ref(false),
  mode = ref<"captcha" | "totp">("captcha");
const isTotp = computed(() => mode.value === "totp");

// ---- 登录页表盘：实时时间与 60 个刻度 ----
const now = ref(new Date());
let clockTimer = 0;
const pad2 = (n: number) => String(n).padStart(2, "0");
const hhmm = computed(() => pad2(now.value.getHours()) + ":" + pad2(now.value.getMinutes()));
const ss = computed(() => pad2(now.value.getSeconds()));
const sec = computed(() => now.value.getSeconds());
const WEEK = ["星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"];
const dateText = computed(() => {
  const d = now.value;
  return d.getFullYear() + "年" + (d.getMonth() + 1) + "月" + d.getDate() + "日 " + WEEK[d.getDay()];
});
const ticks = Array.from({ length: 60 }, (_, i) => {
  const a = ((i * 6 - 90) * Math.PI) / 180;
  const major = i % 5 === 0;
  const r1 = major ? 142 : 150;
  const r2 = 172;
  return {
    i,
    major,
    x1: +(200 + r1 * Math.cos(a)).toFixed(2),
    y1: +(200 + r1 * Math.sin(a)).toFixed(2),
    x2: +(200 + r2 * Math.cos(a)).toFixed(2),
    y2: +(200 + r2 * Math.sin(a)).toFixed(2),
  };
});
const REMEMBER_KEY = "tg_login_remember";
function readRemembered() {
  try {
    const raw = localStorage.getItem(REMEMBER_KEY);
    if (!raw) return;
    const v = JSON.parse(raw);
    if (typeof v?.u === "string" && typeof v?.p === "string") {
      username.value = v.u;
      password.value = v.p;
      remember.value = true;
    }
  } catch {}
}
function saveRemembered() {
  try {
    if (remember.value) {
      localStorage.setItem(
        REMEMBER_KEY,
        JSON.stringify({ u: username.value, p: password.value }),
      );
    } else {
      localStorage.removeItem(REMEMBER_KEY);
    }
  } catch {}
}
let modeSeq = 0;
async function loadMode() {
  const name = username.value.trim();
  const seq = ++modeSeq;
  let next: "captcha" | "totp" = "captcha";
  if (name) {
    try {
      const r: any = await request.get("/auth/login-mode", {
        params: { username: name },
      });
      if (r.data?.mode === "totp") next = "totp";
    } catch {}
  }
  if (seq !== modeSeq) return;
  mode.value = next;
  if (next === "captcha" && !captcha.value) loadCaptcha();
}
async function loadCaptcha() {
  try {
    const r: any = await request.get("/auth/captcha");
    captcha.value = r.data;
  } catch (e: any) {
    captcha.value = null;
    error.value = e.message;
  }
}
function restart() {
  challenge.value = null;
  code.value = "";
  error.value = "";
  password.value = "";
  loadCaptcha();
}
async function submit() {
  busy.value = true;
  error.value = "";
  try {
    if (challenge.value) {
      await user.verifyLogin({
        challengeId: challenge.value.challengeId,
        code: code.value,
      });
      challenge.value = null;
      code.value = "";
      await router.push("/");
    } else {
      const r = await user.login({
        username: username.value,
        password: password.value,
        captchaId: isTotp.value ? "" : captcha.value?.captchaId || "",
        code: code.value,
      });
      saveRemembered();
      password.value = "";
      code.value = "";
      if (r?.verification === "setup") {
        // First-time Google Authenticator binding: show the QR code.
        challenge.value = r;
      } else {
        await router.push("/");
      }
    }
  } catch (e: any) {
    error.value = e.message;
    if (!challenge.value) {
      code.value = "";
      if (!isTotp.value) loadCaptcha();
    }
  } finally {
    busy.value = false;
  }
}
onUnmounted(() => window.clearInterval(clockTimer));
onMounted(() => {
  clockTimer = window.setInterval(() => {
    now.value = new Date();
  }, 500);
  readRemembered();
  loadCaptcha();
  if (username.value) loadMode();
});
</script>
<style>
/* 全局：自定义属性动画与关键帧（scoped 块里引用） */
@property --lg-a {
  syntax: "<angle>";
  inherits: false;
  initial-value: 0deg;
}
@keyframes lg-spin {
  to {
    --lg-a: 360deg;
  }
}
@keyframes lg-orbit {
  to {
    transform: rotate(360deg);
  }
}
@keyframes lg-tick-in {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}
@keyframes lg-rise {
  from {
    opacity: 0;
    transform: translateY(18px) scale(0.985);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
</style>
<style scoped>
.lg-page {
  --lg-ink: #060b1a;
  --lg-text: #e8f0ff;
  --lg-muted: #8fa3c4;
  --lg-teal: #22d3b8;
  --lg-blue: #229ed9;
  --lg-violet: #7c6cf0;
  --lg-mono: "JetBrains Mono", "SF Mono", "Cascadia Mono", ui-monospace, Menlo, Consolas, monospace;
  position: relative;
  isolation: isolate;
  overflow: hidden;
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(420px, 540px);
  min-height: 100vh;
  min-height: 100dvh;
  color: var(--lg-text);
  color-scheme: dark;
  background: var(--lg-ink);
  font-family: "HarmonyOS Sans SC", "PingFang SC", "Noto Sans SC", "Microsoft YaHei", system-ui, sans-serif;
}

/* ---------- 背景：极光 + 网格 ---------- */
.lg-bg {
  position: absolute;
  inset: 0;
  z-index: -1;
  overflow: hidden;
}
.lg-blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(100px);
}
.lg-b1 {
  width: 560px;
  height: 560px;
  left: -160px;
  top: -180px;
  background: #0fb9a1;
  opacity: 0.32;
}
.lg-b2 {
  width: 620px;
  height: 620px;
  right: -200px;
  bottom: -220px;
  background: #6a5cf0;
  opacity: 0.42;
}
.lg-b3 {
  width: 420px;
  height: 420px;
  right: 18%;
  top: -160px;
  background: #1c86cf;
  opacity: 0.28;
}
.lg-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(150, 180, 255, 0.07) 1px, transparent 1px),
    linear-gradient(90deg, rgba(150, 180, 255, 0.07) 1px, transparent 1px);
  background-size: 56px 56px;
  -webkit-mask-image: radial-gradient(70% 65% at 32% 48%, #000 25%, transparent 100%);
  mask-image: radial-gradient(70% 65% at 32% 48%, #000 25%, transparent 100%);
}

/* ---------- 左侧舞台 ---------- */
.lg-stage {
  display: flex;
  flex-direction: column;
  padding: clamp(28px, 5vh, 56px) clamp(32px, 6vw, 96px);
  min-width: 0;
}
.lg-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0.5px;
}
.lg-logo {
  flex: none;
  filter: drop-shadow(0 6px 16px rgba(34, 158, 217, 0.45));
}
.lg-copy {
  margin-top: clamp(28px, 7vh, 80px);
}
.lg-copy h1 {
  margin: 0;
  font-size: clamp(30px, 3.4vw, 54px);
  line-height: 1.32;
  font-weight: 700;
  letter-spacing: 1px;
  color: #f4f8ff;
}
.lg-copy p {
  margin: 18px 0 0;
  max-width: 34em;
  font-size: 14px;
  line-height: 1.9;
  color: var(--lg-muted);
}

/* ---------- 表盘 ---------- */
.lg-dial-wrap {
  --s: min(340px, 36vh);
  position: relative;
  width: var(--s);
  margin: auto 0;
  padding: 28px 0;
}
.lg-dial {
  position: relative;
  width: var(--s);
  height: var(--s);
}
.lg-dial svg {
  width: 100%;
  height: 100%;
  overflow: visible;
}
.lg-ring-in {
  fill: none;
  stroke: rgba(143, 163, 196, 0.16);
  stroke-width: 1;
}
.lg-ring-out {
  fill: none;
  stroke: rgba(143, 163, 196, 0.2);
  stroke-width: 1;
  stroke-dasharray: 2 7;
}
.lg-tick {
  stroke: rgba(143, 163, 196, 0.26);
  stroke-width: 2.2;
  stroke-linecap: round;
  transition: stroke 0.3s;
  animation: lg-tick-in 0.45s both;
  animation-delay: calc(var(--i) * 16ms);
}
.lg-tick.major {
  stroke-width: 3.4;
}
.lg-tick.lit {
  stroke: url(#lg-tick-g);
}
.lg-tick.now {
  stroke: #fff;
  stroke-width: 4;
  filter: drop-shadow(0 0 5px #22d3b8) drop-shadow(0 0 12px rgba(34, 211, 184, 0.7));
}
.lg-orbit {
  transform-origin: 200px 200px;
  animation: lg-orbit 26s linear infinite;
}
.lg-plane {
  fill: #bff7ee;
  filter: drop-shadow(0 0 6px rgba(34, 211, 184, 0.9));
}
.lg-readout {
  position: absolute;
  inset: 0;
  display: grid;
  place-content: center;
  text-align: center;
}
.lg-time {
  display: flex;
  align-items: baseline;
  justify-content: center;
  gap: 4px;
  font-family: var(--lg-mono);
  font-variant-numeric: tabular-nums;
  color: #f4f8ff;
  line-height: 1;
}
.lg-time span {
  font-size: calc(var(--s) * 0.19);
  font-weight: 600;
  letter-spacing: -0.02em;
}
.lg-time em {
  font-style: normal;
  font-size: calc(var(--s) * 0.085);
  font-weight: 500;
  color: var(--lg-teal);
}
.lg-date {
  margin-top: calc(var(--s) * 0.04);
  font-size: max(11px, calc(var(--s) * 0.04));
  letter-spacing: 1px;
  color: var(--lg-muted);
}

/* 表盘旁的功能提示 */
.lg-chip {
  position: absolute;
  display: inline-flex;
  align-items: center;
  gap: 9px;
  padding: 8px 15px 8px 12px;
  border-radius: 999px;
  font-size: 12.5px;
  white-space: nowrap;
  color: #d5e2fb;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.12);
  -webkit-backdrop-filter: blur(10px);
  backdrop-filter: blur(10px);
}
.lg-chip i {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--c);
  box-shadow: 0 0 10px var(--c);
}
.lg-chip-a {
  --c: #3ddc97;
  left: calc(100% + 26px);
  top: 16%;
}
.lg-chip-b {
  --c: #4cb8ff;
  left: calc(100% + 58px);
  top: 46%;
}
.lg-chip-c {
  --c: #ffb454;
  left: calc(100% + 18px);
  top: 76%;
}
.lg-foot {
  font-size: 12px;
  color: #6f86a8;
}

/* ---------- 右侧登录卡 ---------- */
.lg-side {
  display: grid;
  place-items: center;
  min-width: 0;
  padding: 40px clamp(20px, 4vw, 64px);
}
.lg-card {
  position: relative;
  width: 100%;
  min-width: 0;
  max-width: 430px;
  padding: 38px 36px 30px;
  border-radius: 26px;
  background: rgba(10, 16, 40, 0.66);
  -webkit-backdrop-filter: blur(22px) saturate(150%);
  backdrop-filter: blur(22px) saturate(150%);
  box-shadow:
    0 40px 90px -36px rgba(0, 0, 0, 0.85),
    0 0 0 1px rgba(255, 255, 255, 0.06) inset;
  animation: lg-rise 0.9s cubic-bezier(0.2, 0.8, 0.2, 1) 0.2s both;
}
.lg-card::before {
  content: "";
  position: absolute;
  inset: -1px;
  padding: 1px;
  border-radius: 27px;
  background: conic-gradient(
    from var(--lg-a),
    rgba(34, 211, 184, 0) 0 52%,
    #22d3b8 70%,
    #229ed9 83%,
    #8b7cff 94%,
    rgba(139, 124, 255, 0)
  );
  -webkit-mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
  -webkit-mask-composite: xor;
  mask: linear-gradient(#000 0 0) content-box exclude, linear-gradient(#000 0 0);
  pointer-events: none;
  animation: lg-spin 7s linear infinite;
}
.lg-card-head {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 30px;
}
.lg-card-head h2 {
  margin: 0 0 6px;
  font-size: 23px;
  font-weight: 700;
  color: #f4f8ff;
  letter-spacing: 0.5px;
}
.lg-muted {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--lg-muted);
}
.lg-note {
  margin-top: 22px;
  font-size: 11.5px;
  color: #6f86a8;
}

/* ---------- 表单（深色） ---------- */
.verification-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}
.verification-row :deep(.el-input) {
  flex: 1;
  min-width: 0;
}
.verification-row img {
  flex-shrink: 0;
  height: 46px;
  width: auto;
  border-radius: 12px;
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.14);
}
.login-fields :deep(.el-form-item) {
  margin-bottom: 18px;
  min-width: 0;
}
.login-fields :deep(.el-form-item__content) {
  min-width: 0;
}
.lg-fields :deep(.el-form-item__label) {
  color: #b9c7e3;
}
.lg-fields :deep(.el-input__wrapper) {
  min-height: 46px;
  padding: 0 16px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.05);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.12) inset;
  transition: box-shadow 0.2s, background-color 0.2s;
}
.lg-fields :deep(.el-input__wrapper:hover) {
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.24) inset;
}
.lg-fields :deep(.el-input__wrapper.is-focus) {
  background: rgba(34, 211, 184, 0.07);
  box-shadow:
    0 0 0 1.5px #22d3b8 inset,
    0 0 0 4px rgba(34, 211, 184, 0.16);
}
.lg-fields :deep(.el-input__inner) {
  color: var(--lg-text);
  font-size: 15px;
}
.lg-fields :deep(.el-input__inner::placeholder) {
  color: #667b9e;
}
.lg-fields :deep(.el-input__prefix),
.lg-fields :deep(.el-input__prefix .el-icon),
.lg-fields :deep(.el-input__suffix),
.lg-fields :deep(.el-input__suffix .el-icon) {
  color: #7d92b5;
}
/* 浏览器自动填充会强制白底，这里压回深色 */
.lg-fields :deep(.el-input__inner:-webkit-autofill) {
  -webkit-text-fill-color: var(--lg-text);
  box-shadow: 0 0 0 1000px #101a3c inset;
  caret-color: var(--lg-text);
  border-radius: 12px;
}
.remember-box {
  margin: -2px 0 2px;
  color: var(--lg-muted);
}
.lg-fields :deep(.el-checkbox__label) {
  color: var(--lg-muted);
}
.lg-fields :deep(.el-checkbox__inner) {
  background: transparent;
  border-color: #3b4c78;
}
.lg-fields :deep(.el-checkbox__input.is-checked .el-checkbox__inner) {
  background: #22d3b8;
  border-color: #22d3b8;
}
.lg-fields :deep(.el-checkbox__input.is-checked .el-checkbox__inner::after) {
  border-color: #04131c;
}
.lg-fields :deep(.el-checkbox__input.is-checked + .el-checkbox__label) {
  color: var(--lg-text);
}
.lg-fields :deep(.el-button.is-link) {
  color: #7fe3d3;
}
.lg-fields :deep(.el-alert--error.is-light) {
  margin-top: 4px;
  border: 1px solid rgba(255, 107, 129, 0.32);
  border-radius: 12px;
  background: rgba(255, 107, 129, 0.12);
}
.lg-fields :deep(.el-alert--error.is-light .el-alert__title),
.lg-fields :deep(.el-alert--error.is-light .el-alert__icon) {
  color: #ffb3bf;
}
.totp-setup {
  text-align: center;
}
.totp-setup p {
  font-size: 13px;
  line-height: 1.7;
  color: #b9c7e3;
}
.totp-setup p.lg-muted {
  color: var(--lg-muted);
}
.totp-setup > img {
  display: block;
  margin: 12px auto;
  padding: 10px;
  border-radius: 16px;
  background: #fff; /* 二维码需要白底才能被扫描 */
}

/* 登录按钮 */
.lg-fields :deep(.lg-submit.el-button) {
  position: relative;
  overflow: hidden;
  display: flex;
  width: 100%;
  height: 48px;
  margin: 22px 0 0;
  border: 0;
  border-radius: 14px;
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 2px;
  color: #04131c;
  background: linear-gradient(110deg, #22d3b8, #229ed9 52%, #8b7cff);
  box-shadow: 0 12px 30px -12px rgba(34, 211, 184, 0.7);
  transition: transform 0.15s, box-shadow 0.2s;
}
.lg-fields :deep(.lg-submit.el-button::after) {
  content: "";
  position: absolute;
  inset: 0;
  background: linear-gradient(105deg, transparent 30%, rgba(255, 255, 255, 0.5) 50%, transparent 70%);
  transform: translateX(-120%);
  pointer-events: none;
}
.lg-fields :deep(.lg-submit.el-button:hover) {
  transform: translateY(-1px);
  box-shadow: 0 16px 36px -12px rgba(34, 211, 184, 0.85);
}
.lg-fields :deep(.lg-submit.el-button:hover::after) {
  transform: translateX(120%);
  transition: transform 0.7s;
}
.lg-fields :deep(.lg-submit.el-button:focus-visible) {
  outline: 2px solid #fff;
  outline-offset: 3px;
}

/* ---------- 响应式 ---------- */
@media (max-width: 1240px) {
  .lg-chip {
    display: none;
  }
}
@media (max-width: 980px) {
  .lg-page {
    grid-template-columns: minmax(0, 1fr);
  }
  .lg-stage {
    display: none;
  }
  .lg-side {
    min-height: 100vh;
    min-height: 100dvh;
    padding: 24px 16px;
  }
  .lg-card {
    padding: 32px 24px 26px;
  }
}
@media (prefers-reduced-motion: reduce) {
  .lg-card,
  .lg-card::before,
  .lg-orbit,
  .lg-tick {
    animation: none;
  }
}
</style>
