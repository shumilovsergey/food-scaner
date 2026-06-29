// ════════════════════════════════════════════════════════════════════════
// food-scaner — client logic
// Chrome (profile popover, tabs) + app logic. All data comes from /api/* — no
// hardcoded meals/favorites. The app block only runs on the logged-in screen.
// ════════════════════════════════════════════════════════════════════════

// ── Profile button: initial letter + popover toggle ─────────────────────────
const profileBtn = document.getElementById('profileBtn');
if (profileBtn) {
  const name = profileBtn.dataset.name || '';
  const initial = profileBtn.querySelector('.profile-initial');
  if (initial) initial.textContent = (name[0] || '?').toUpperCase();
  const popover = document.getElementById('profilePopover');
  profileBtn.addEventListener('click', () => {
    const open = popover.classList.toggle('open');
    profileBtn.classList.toggle('open', open);
  });
  document.addEventListener('click', e => {
    if (!profileBtn.contains(e.target) && !popover.contains(e.target)) {
      popover.classList.remove('open');
      profileBtn.classList.remove('open');
    }
  });
}

// ── App logic (only on the scanner screen) ──────────────────────────────────
(function app() {
  if (!document.querySelector('.screen')) return; // logged-out: nothing to wire

  // ── tiny fetch wrapper: throws {status, message} on non-2xx ──
  async function api(method, url, body) {
    const opt = { method, headers: {} };
    if (body !== undefined) {
      opt.headers['Content-Type'] = 'application/json';
      opt.body = JSON.stringify(body);
    }
    const res = await fetch(url, opt);
    let data = null;
    try { data = await res.json(); } catch (_) {}
    if (!res.ok) {
      const err = new Error((data && data.error) || 'Ошибка');
      err.status = res.status;
      throw err;
    }
    return data;
  }

  // ── client cache, hydrated from /api/state ──
  let MEALS = [];
  let FAVORITES = [];
  let DONUT = { kcal: 0, prot: 0 };

  async function loadState() {
    const s = await api('GET', '/api/state');
    MEALS = s.meals || [];
    FAVORITES = s.favorites || [];
    DONUT = s.donut || { kcal: 0, prot: 0 };
    const cs = document.getElementById('c-status');
    const cu = document.getElementById('c-uses');
    if (cs) cs.textContent = s.status || '—';
    if (cu) cu.textContent = s.usesToday ?? 0;
    drawHistory();
    drawToday();
    snapTableToBottom();
  }

  // ── today's donut: two rings (calories outer, protein inner) ──
  const CAL_C = 2 * Math.PI * 52;
  const PROT_C = 2 * Math.PI * 39;
  const CAL_NORM = 2000, PROT_NORM = 120; // display scale, not a personal goal

  function drawToday() {
    const kcal = Number(DONUT.kcal) || 0;
    const prot = Number(DONUT.prot) || 0;
    const calFrac = Math.min(1, kcal / CAL_NORM);
    const protFrac = Math.min(1, prot / PROT_NORM);
    document.getElementById('d-kcal').textContent = kcal.toLocaleString();
    document.getElementById('d-prot').textContent = Math.round(prot).toLocaleString();
    document.getElementById('d-cal-arc')
      .setAttribute('stroke-dasharray', `${(calFrac * CAL_C).toFixed(1)} ${CAL_C.toFixed(1)}`);
    document.getElementById('d-prot-arc')
      .setAttribute('stroke-dasharray', `${(protFrac * PROT_C).toFixed(1)} ${PROT_C.toFixed(1)}`);
  }

  // ── meals table — today only, newest first ──
  function esc(s) {
    return String(s).replace(/[&<>"']/g, c =>
      ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  function drawHistory() {
    const wrap = document.getElementById('history-rows');
    if (!MEALS.length) {
      wrap.innerHTML = `<div class="empty">No meals yet today — add your first above.</div>`;
      return;
    }
    wrap.innerHTML = MEALS.map(m => `
      <div class="row" data-id="${m.id}">
        <span class="meal">
          <span class="name">${m.fav ? '★ ' : ''}${esc(m.name)}</span>
          <span class="time">${esc(m.time || '')}</span>
        </span>
        <span class="kcal">${Number(m.kcal).toLocaleString()}</span>
      </div>`).join('');
  }

  // ── meal card popup (reusable add/edit) ──
  let editingId = null; // null = adding a new meal

  function openMeal(meal) {
    editingId = meal && meal.id ? meal.id : null;
    const g = id => document.getElementById(id);
    g('m-name').value = meal ? (meal.name || '') : '';
    g('m-kcal').value = meal && meal.kcal != null ? meal.kcal : '';
    g('m-grams').value = meal && meal.grams != null ? meal.grams : '';
    g('m-prot').value = meal && meal.prot != null ? meal.prot : '';
    g('m-fat').value = meal && meal.fat != null ? meal.fat : '';
    g('m-carb').value = meal && meal.carb != null ? meal.carb : '';
    g('meal-fav').classList.toggle('on', !!(meal && meal.fav));
    g('meal-del').style.display = editingId ? 'flex' : 'none';
    document.getElementById('overlay').classList.add('show');
  }

  function closeMeal() {
    document.getElementById('overlay').classList.remove('show');
    editingId = null;
  }

  function mealPayload() {
    const num = id => Number(document.getElementById(id).value) || 0;
    return {
      name: document.getElementById('m-name').value.trim() || 'Meal',
      kcal: num('m-kcal'), grams: num('m-grams'),
      prot: num('m-prot'), fat: num('m-fat'), carb: num('m-carb'),
    };
  }

  async function saveMeal() {
    const data = mealPayload();
    const fav = document.getElementById('meal-fav').classList.contains('on');
    try {
      if (editingId) await api('POST', `/api/meal/${editingId}`, data);
      else await api('POST', '/api/meal', data);
      if (fav) await api('POST', '/api/favorite', data); // star upserts the template
      closeMeal();
      await loadState();
    } catch (e) { toast(e.message); }
  }

  async function deleteMeal() {
    if (editingId == null) return;
    try {
      await api('DELETE', `/api/meal/${editingId}`);
      closeMeal();
      await loadState();
      toast('Meal removed');
    } catch (e) { toast(e.message); }
  }

  // ── favorites picker (pick one saved meal -> eat it today) ──
  let favSelected = null; // index into FAVORITES, or null

  function openFav() {
    favSelected = null;
    drawFav();
    document.getElementById('overlay-fav').classList.add('show');
  }

  function drawFav() {
    const rows = document.getElementById('fav-rows');
    if (!FAVORITES.length) {
      rows.innerHTML = `<div class="empty">No favorites yet — star a meal to save it here.</div>`;
      return;
    }
    rows.innerHTML = FAVORITES.map((f, i) => `
      <div class="fav-row${i === favSelected ? ' sel' : ''}" data-i="${i}">
        <span class="fname">${esc(f.name)}</span>
        <span class="fg">${f.grams} g</span>
        <span class="fk">${Number(f.kcal).toLocaleString()}</span>
      </div>`).join('');
  }

  async function deleteFav() {
    if (favSelected == null) { toast('Pick a meal first'); return; }
    const f = FAVORITES[favSelected];
    try {
      await api('DELETE', `/api/favorite/${f.id}`);
      favSelected = null;
      await loadState();
      drawFav(); // keep the picker open
    } catch (e) { toast(e.message); }
  }

  async function acceptFav() {
    if (favSelected == null) { toast('Pick a meal first'); return; }
    const f = FAVORITES[favSelected];
    try {
      await api('POST', '/api/meal', {
        name: f.name, kcal: f.kcal, grams: f.grams, prot: f.prot, fat: f.fat, carb: f.carb,
      });
      closeOverlay('overlay-fav');
      await loadState();
    } catch (e) { toast(e.message); }
  }

  // ── AI scan (photo + text) → prefill the meal sheet for review ──
  async function runScan(payload, btn) {
    if (btn) btn.disabled = true;
    toast('Анализирую…');
    try {
      const result = await api('POST', '/api/scan', payload);
      return result;
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  // ── toast ──
  let toastT;
  function toast(msg) {
    const el = document.getElementById('toast');
    el.textContent = msg;
    el.classList.add('show');
    clearTimeout(toastT);
    toastT = setTimeout(() => el.classList.remove('show'), 1800);
  }

  // ── header: pill tabs ──
  document.getElementById('navTabs').addEventListener('click', e => {
    const tab = e.target.closest('.tab');
    if (!tab) return;
    document.querySelectorAll('#navTabs .tab').forEach(t => t.classList.remove('active'));
    tab.classList.add('active');
    const which = tab.dataset.tab;
    document.getElementById('panel-scaner').hidden = which !== 'scaner';
    document.getElementById('panel-credits').hidden = which !== 'credits';
    if (which === 'scaner') snapTableToBottom();
  });

  // ── sources row ──
  document.getElementById('sources').addEventListener('click', e => {
    const btn = e.target.closest('.source');
    if (!btn) return;
    const src = btn.dataset.src;
    if (src === 'manual') { openMeal(null); return; }
    if (src === 'photo') { openOverlay('overlay-photo'); return; }
    if (src === 'text') { openOverlay('overlay-text'); return; }
    openFav();
  });

  // ── favorites picker wiring ──
  document.getElementById('fav-rows').addEventListener('click', e => {
    const row = e.target.closest('.fav-row');
    if (!row) return;
    favSelected = Number(row.dataset.i);
    drawFav();
  });
  document.getElementById('fav-accept').addEventListener('click', acceptFav);
  document.getElementById('fav-del').addEventListener('click', deleteFav);
  document.getElementById('overlay-fav').addEventListener('click', e => {
    if (e.target.id === 'overlay-fav' || e.target.closest('[data-close-fav]')) closeOverlay('overlay-fav');
  });

  // ── AI photo: live camera, capture, gallery ──
  let camStream = null;
  async function startCamera() {
    const vf = document.getElementById('viewfinder');
    const video = document.getElementById('cam');
    try {
      camStream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' }, audio: false });
      video.srcObject = camStream;
      await video.play();
      vf.classList.add('live');
    } catch (_) {
      vf.classList.remove('live'); // denied / no camera / insecure context
    }
  }
  function stopCamera() {
    const vf = document.getElementById('viewfinder');
    const video = document.getElementById('cam');
    vf.classList.remove('live');
    if (camStream) { camStream.getTracks().forEach(t => t.stop()); camStream = null; }
    if (video) video.srcObject = null;
  }

  // capture the current video frame as base64 jpeg (no data: prefix)
  function captureFrame() {
    const video = document.getElementById('cam');
    if (!video.videoWidth) return null;
    const canvas = document.createElement('canvas');
    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
    canvas.getContext('2d').drawImage(video, 0, 0);
    return canvas.toDataURL('image/jpeg', 0.85).split(',')[1];
  }

  async function scanPhoto(base64, btn) {
    try {
      const result = await runScan({ mode: 'photo', image: base64, media_type: 'image/jpeg' }, btn);
      closeOverlay('overlay-photo');
      openMeal(result); // prefilled, adding — user reviews and saves
    } catch (e) { toast(e.message); }
  }

  document.getElementById('photo-take').addEventListener('click', e => {
    const base64 = captureFrame();
    if (!base64) { toast('Камера недоступна — выбери фото из галереи'); return; }
    scanPhoto(base64, e.currentTarget);
  });

  document.getElementById('photo-file').addEventListener('change', e => {
    const file = e.target.files && e.target.files[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      const base64 = String(reader.result).split(',')[1];
      scanPhoto(base64, null);
    };
    reader.readAsDataURL(file);
    e.target.value = ''; // allow re-picking the same file
  });

  // ── AI text ──
  document.getElementById('text-send').addEventListener('click', async e => {
    const ta = document.getElementById('text-desc');
    const desc = ta.value.trim();
    if (!desc) { toast('Опиши блюдо'); return; }
    try {
      const result = await runScan({ mode: 'text', text: desc }, e.currentTarget);
      closeOverlay('overlay-text');
      openMeal(result);
    } catch (err) { toast(err.message); }
  });

  // ── generic overlays (text/photo) ──
  function openOverlay(id) {
    document.getElementById(id).classList.add('show');
    if (id === 'overlay-photo') startCamera();
  }
  function closeOverlay(id) {
    document.getElementById(id).classList.remove('show');
    if (id === 'overlay-photo') stopCamera();
    if (id === 'overlay-text') document.getElementById('text-desc').value = '';
  }
  document.querySelectorAll('#overlay-text, #overlay-photo').forEach(ov => {
    ov.addEventListener('click', e => {
      if (e.target === ov || e.target.closest('[data-close]')) closeOverlay(ov.id);
    });
  });

  // ── meal sheet wiring ──
  document.getElementById('history-rows').addEventListener('click', e => {
    const row = e.target.closest('.row');
    if (!row) return;
    const m = MEALS.find(x => x.id === Number(row.dataset.id));
    if (m) openMeal(m);
  });
  document.getElementById('meal-close').addEventListener('click', closeMeal);
  document.getElementById('meal-save').addEventListener('click', saveMeal);
  document.getElementById('meal-del').addEventListener('click', deleteMeal);
  document.getElementById('meal-fav').addEventListener('click', e => e.currentTarget.classList.toggle('on'));
  document.getElementById('overlay').addEventListener('click', e => {
    if (e.target.id === 'overlay') closeMeal();
  });

  // ── bottom-snap (skeleton/bottom_snap.md) ──
  // The donut card is the flexible element (the doc's "graph"). We grow it to absorb the
  // free space so the meals table HEADER snaps to the very bottom edge — it just peeks, so
  // the user sees a table exists and can scroll up to reveal today's rows, while the donut
  // stays big and airy. Pinning the header (not the whole list) is what keeps the donut
  // from collapsing. Measured, so it adapts to any resolution and re-runs on resize/rotate.
  function snapTableToBottom() {
    const panel = document.getElementById('panel-scaner');
    if (panel.hidden) return;                                  // only on the active panel
    const card = document.getElementById('donut-card');        // flexible element
    const screen = document.querySelector('.screen');          // scroll column
    const anchor = panel.querySelector('#history .table-head'); // edge to pin: the table "hat"

    card.style.height = '';                                    // 1) reset to natural minimum

    // 2) anchor bottom, measured from the column top — scroll-independent
    const anchorBottom = anchor.getBoundingClientRect().bottom - screen.getBoundingClientRect().top;

    // 3) pour the leftover space into the donut card; visualViewport (not clientHeight) so
    //    the mobile toolbar is accounted for. Clamp >= 0 → short screens just scroll.
    const vh = (window.visualViewport && window.visualViewport.height) || document.documentElement.clientHeight;
    const slack = vh - anchorBottom;
    if (slack > 0) card.style.height = (card.offsetHeight + slack) + 'px';
  }
  window.addEventListener('resize', snapTableToBottom);
  window.addEventListener('orientationchange', snapTableToBottom);
  if (window.visualViewport) window.visualViewport.addEventListener('resize', snapTableToBottom);

  // ── init ──
  drawHistory();
  drawToday();
  requestAnimationFrame(snapTableToBottom);
  loadState().catch(e => toast(e.message));
})();
