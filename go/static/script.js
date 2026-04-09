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

let currentUser = null;

function showScreen(screen) {
  [loginScreen, pendingScreen, mainScreen].forEach(s => s.classList.add('hidden'));
  screen.classList.remove('hidden');
}

async function init() {
  try {
    const res = await fetch('/api/me');
    if (res.status === 401) { showScreen(loginScreen); return; }
    currentUser = await res.json();
    if (!currentUser.approved) {
      pendingAuthId.textContent = currentUser.auth_id;
      showScreen(pendingScreen);
      return;
    }
    showScreen(mainScreen);
    startCamera();
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

  canvas.width  = video.videoWidth;
  canvas.height = video.videoHeight;
  canvas.getContext('2d').drawImage(video, 0, 0);
  const base64 = canvas.toDataURL('image/jpeg', 0.85).split(',')[1];

  scanBtn.disabled = true;
  resultSheet.classList.add('hidden');
  loadingOverlay.classList.remove('hidden');

  try {
    const res = await fetch('/api/scan', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ image: base64, media_type: 'image/jpeg' }),
    });

    if (res.status === 401) { location.href = '/login'; return; }
    if (res.status === 403) { showScreen(pendingScreen); return; }
    if (res.status === 429) { alert((await res.text()).trim()); return; }
    if (res.status === 402) { alert('Средства на счёте закончились. Свяжитесь с Сергеем Шумиловым.'); return; }
    if (!res.ok) throw new Error(await res.text() || res.statusText);

    const data = await res.json();

    resultName.textContent     = data.name;
    resultCalories.textContent = Math.round(data.calories);
    resultGrams.textContent    = Math.round(data.grams);
    resultProteins.textContent = Math.round(data.proteins);
    resultFats.textContent     = Math.round(data.fats);
    resultCarbs.textContent    = Math.round(data.carbs);

    // refresh user info
    const me = await fetch('/api/me').then(r => r.json()).catch(() => currentUser);
    currentUser = me;
    const left = Math.max(0, me.daily_limit - me.today_scans);
    resultAuthId.textContent    = me.auth_id;
    resultScansLeft.textContent = `${left} из ${me.daily_limit}`;

    resultSheet.classList.remove('hidden');
  } catch (err) {
    alert('Ошибка: ' + err.message);
  } finally {
    loadingOverlay.classList.add('hidden');
    scanBtn.disabled = false;
  }
});

closeResultBtn.addEventListener('click', () => resultSheet.classList.add('hidden'));
logoutBtnPending.addEventListener('click', () => { location.href = '/logout'; });

init();
