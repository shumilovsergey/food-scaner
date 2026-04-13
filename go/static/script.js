const loginScreen     = document.getElementById('login-screen');
const pendingScreen   = document.getElementById('pending-screen');
const mainScreen      = document.getElementById('main-screen');
const video           = document.getElementById('video');
const canvas          = document.getElementById('canvas');
const scanBtn         = document.getElementById('scan-btn');
const loadingOverlay  = document.getElementById('loading-overlay');
const resultSheet     = document.getElementById('result-sheet');
const closeResultBtn  = document.getElementById('close-result-btn');
const pendingAuthId   = document.getElementById('pending-auth-id');

const resultName      = document.getElementById('result-name');
const resultCalories  = document.getElementById('result-calories');
const resultGrams     = document.getElementById('result-grams');
const resultProteins  = document.getElementById('result-proteins');
const resultFats      = document.getElementById('result-fats');
const resultCarbs     = document.getElementById('result-carbs');
const resultAuthId    = document.getElementById('result-auth-id');
const resultScansLeft = document.getElementById('result-scans-left');
const logoutBtnPending = document.getElementById('logout-btn-pending');
const galleryBtn       = document.getElementById('gallery-btn');
const galleryInput     = document.getElementById('gallery-input');
const userBtn          = document.getElementById('user-btn');
const userSheet        = document.getElementById('user-sheet');
const closeUserBtn     = document.getElementById('close-user-btn');
const logoutBtn        = document.getElementById('logout-btn');
const userSheetName    = document.getElementById('user-sheet-name');
const userSheetId      = document.getElementById('user-sheet-id');
const userSheetStatus  = document.getElementById('user-sheet-status');
const userSheetUsed    = document.getElementById('user-sheet-used');
const userSheetBuy     = document.getElementById('user-sheet-buy');
const userBuyScansBtn  = document.getElementById('user-buy-scans-btn');
const userBuyProBtn    = document.getElementById('user-buy-pro-btn');

const welcomeSheet     = document.getElementById('welcome-sheet');
const closeWelcomeBtn  = document.getElementById('close-welcome-btn');
const noScansSheet     = document.getElementById('no-scans-sheet');
const closeNoScansBtn  = document.getElementById('close-no-scans-btn');
const buyScansBtn      = document.getElementById('buy-scans-btn');
const buyProBtn        = document.getElementById('buy-pro-btn');

let currentUser = null;

function showScreen(screen) {
  [loginScreen, pendingScreen, mainScreen].forEach(s => s.classList.add('hidden'));
  screen.classList.remove('hidden');
}

function canScanClient() {
  if (!currentUser) return false;
  const r = currentUser.role;
  if (r === 'pro') return true;
  if (r === 'tester') return true;
  if (currentUser.owned_scans > 0) return true;
  if (currentUser.free_scans_left > 0) return true;
  return false;
}

function scansLeftText(me) {
  if (me.role === 'pro') return 'PRO — безлимит';
  if (me.role === 'tester') {
    const left = Math.max(0, me.daily_limit - me.today_scans);
    return `${left} из ${me.daily_limit} сегодня`;
  }
  const total = (me.free_scans_left || 0) + (me.owned_scans || 0);
  return `${total} осталось`;
}

function statusText(me) {
  if (me.role === 'pro') return `PRO до ${me.pro_until}`;
  if (me.role === 'tester') return `Тестер · ${me.daily_limit}/день`;
  const free = me.free_scans_left || 0;
  const owned = me.owned_scans || 0;
  if (free === 0 && owned === 0) return 'Сканы закончились';
  const parts = [];
  if (free > 0) parts.push(`${free} бесплатных`);
  if (owned > 0) parts.push(`${owned} купленных`);
  return parts.join(' · ');
}

async function init() {
  try {
    const res = await fetch('/api/me');
    if (res.status === 401) { showScreen(loginScreen); return; }
    currentUser = await res.json();
    showScreen(mainScreen);
    startCamera();

    // Show welcome sheet once for new free users
    if (currentUser.role === 'free' &&
        currentUser.total_scans === 0 &&
        !localStorage.getItem('welcomed')) {
      welcomeSheet.classList.remove('hidden');
    }
  } catch {
    showScreen(loginScreen);
  }
}

async function startCamera() {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({
      video: { facingMode: 'environment', width: { ideal: 1280 }, height: { ideal: 720 } },
      audio: false,
    });
    video.srcObject = stream;
  } catch (err) {
    alert('Нет доступа к камере: ' + err.message);
  }
}

scanBtn.addEventListener('click', async () => {
  if (scanBtn.disabled) return;
  if (!canScanClient()) {
    noScansSheet.classList.remove('hidden');
    return;
  }
  canvas.width  = video.videoWidth;
  canvas.height = video.videoHeight;
  canvas.getContext('2d').drawImage(video, 0, 0);
  const base64 = canvas.toDataURL('image/jpeg', 0.85).split(',')[1];
  await doScan(base64, 'image/jpeg');
});

async function doScan(base64, mediaType) {
  scanBtn.disabled = true;
  resultSheet.classList.add('hidden');
  userSheet.classList.add('hidden');
  welcomeSheet.classList.add('hidden');
  noScansSheet.classList.add('hidden');
  loadingOverlay.classList.remove('hidden');

  try {
    const res = await fetch('/api/scan', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ image: base64, media_type: mediaType }),
    });

    if (res.status === 401) { location.href = '/login'; return; }
    if (res.status === 403) { noScansSheet.classList.remove('hidden'); return; }
    if (res.status === 429) { alert((await res.text()).trim()); return; }
    if (res.status === 402) {
      if (currentUser && (currentUser.role === 'tester' || currentUser.role === 'pro')) {
        alert('Средства на счёте закончились. Свяжитесь с Сергеем Шумиловым.');
      } else {
        alert('Сервис временно недоступен. Попробуйте позже.');
      }
      return;
    }
    if (!res.ok) throw new Error(await res.text() || res.statusText);

    const data = await res.json();
    resultName.textContent     = data.name;
    resultCalories.textContent = Math.round(data.calories);
    resultGrams.textContent    = Math.round(data.grams);
    resultProteins.textContent = Math.round(data.proteins);
    resultFats.textContent     = Math.round(data.fats);
    resultCarbs.textContent    = Math.round(data.carbs);

    const me = await fetch('/api/me').then(r => r.json()).catch(() => currentUser);
    currentUser = me;
    resultAuthId.textContent    = me.auth_id;
    resultScansLeft.textContent = scansLeftText(me);

    resultSheet.classList.remove('hidden');
  } catch (err) {
    alert('Ошибка: ' + err.message);
  } finally {
    loadingOverlay.classList.add('hidden');
    scanBtn.disabled = false;
  }
}

closeResultBtn.addEventListener('click', () => resultSheet.classList.add('hidden'));
logoutBtnPending.addEventListener('click', () => { location.href = '/logout'; });

// ── Gallery ──
galleryBtn.addEventListener('click', () => galleryInput.click());
galleryInput.addEventListener('change', async () => {
  const file = galleryInput.files[0];
  if (!file) return;
  galleryInput.value = '';

  const base64 = await new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result.split(',')[1]);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });

  await doScan(base64, file.type || 'image/jpeg');
});

// ── User info sheet ──
userBtn.addEventListener('click', () => {
  if (!currentUser) return;
  userSheetName.textContent   = currentUser.username || '—';
  userSheetId.textContent     = currentUser.auth_id;
  userSheetStatus.textContent = statusText(currentUser);
  userSheetUsed.textContent   = currentUser.today_scans;
  userSheetBuy.classList.toggle('hidden', currentUser.role === 'pro');
  userSheet.classList.remove('hidden');
});

closeUserBtn.addEventListener('click', () => userSheet.classList.add('hidden'));
logoutBtn.addEventListener('click', () => { location.href = '/logout'; });

// ── Welcome sheet ──
closeWelcomeBtn.addEventListener('click', () => {
  localStorage.setItem('welcomed', '1');
  welcomeSheet.classList.add('hidden');
});

// ── No-scans sheet ──
closeNoScansBtn.addEventListener('click', () => noScansSheet.classList.add('hidden'));

const shopMsg = 'Магазин еще не открылся';
buyScansBtn.addEventListener('click', () => alert(shopMsg));
buyProBtn.addEventListener('click', () => alert(shopMsg));
userBuyScansBtn.addEventListener('click', () => alert(shopMsg));
userBuyProBtn.addEventListener('click', () => alert(shopMsg));

init();
