const loginScreen = document.getElementById('login-screen');
const pendingScreen = document.getElementById('pending-screen');
const mainScreen = document.getElementById('main-screen');
const video = document.getElementById('video');
const canvas = document.getElementById('canvas');
const scanBtn = document.getElementById('scan-btn');
const loadingOverlay = document.getElementById('loading-overlay');
const resultCard = document.getElementById('result-card');
const pendingAuthId = document.getElementById('pending-auth-id');
const closeResultBtn = document.getElementById('close-result-btn');
const resultName = document.getElementById('result-name');
const resultGrams = document.getElementById('result-grams');
const resultCalories = document.getElementById('result-calories');
const scanCounter = document.getElementById('scan-counter');
const logoutBtn = document.getElementById('logout-btn');
const logoutBtnPending = document.getElementById('logout-btn-pending');

function showScreen(screen) {
  [loginScreen, pendingScreen, mainScreen].forEach(s => s.classList.add('hidden'));
  screen.classList.remove('hidden');
}

async function init() {
  try {
    const res = await fetch('/api/me');
    if (res.status === 401) {
      showScreen(loginScreen);
      return;
    }
    const user = await res.json();
    if (!user.approved) {
      pendingAuthId.textContent = user.auth_id;
      showScreen(pendingScreen);
      return;
    }
    showScreen(mainScreen);
    scanCounter.textContent = `${user.today_scans}/${user.daily_limit} today`;
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
    alert('Camera access denied: ' + err.message);
  }
}

scanBtn.addEventListener('click', async () => {
  if (scanBtn.disabled) return;

  canvas.width = video.videoWidth;
  canvas.height = video.videoHeight;
  canvas.getContext('2d').drawImage(video, 0, 0);

  const base64 = canvas.toDataURL('image/jpeg', 0.85).split(',')[1];

  scanBtn.disabled = true;
  resultCard.classList.add('hidden');
  loadingOverlay.classList.remove('hidden');

  try {
    const res = await fetch('/api/scan', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ image: base64, media_type: 'image/jpeg' }),
    });

    if (res.status === 401) { location.href = '/login'; return; }
    if (res.status === 403) { showScreen(pendingScreen); return; }
    if (res.status === 429) {
      const text = await res.text();
      alert(text.trim());
      return;
    }
    if (!res.ok) {
      const text = await res.text();
      throw new Error(text || res.statusText);
    }

    const data = await res.json();
    resultName.textContent = data.name;
    resultGrams.textContent = Math.round(data.grams);
    resultCalories.textContent = Math.round(data.calories);
    resultCard.classList.remove('hidden');

    // refresh counter
    const me = await fetch('/api/me').then(r => r.json()).catch(() => null);
    if (me) scanCounter.textContent = `${me.today_scans}/${me.daily_limit} today`;
  } catch (err) {
    alert('Error: ' + err.message);
  } finally {
    loadingOverlay.classList.add('hidden');
    scanBtn.disabled = false;
  }
});

closeResultBtn.addEventListener('click', () => resultCard.classList.add('hidden'));

logoutBtn.addEventListener('click', () => { location.href = '/logout'; });
logoutBtnPending.addEventListener('click', () => { location.href = '/logout'; });

init();
