// All API calls use relative URLs; nginx routes /api/v1/images → api-service,
// /api/v1/jobs → job-service.

const $ = id => document.getElementById(id);

let imageId = null;
let jobId   = null;
let pollTimer = null;

const sectionUpload = $('section-upload');
const sectionJob    = $('section-job');
const sectionStatus = $('section-status');

const dropZone  = $('drop-zone');
const fileInput = $('file-input');

dropZone.addEventListener('click', () => fileInput.click());
dropZone.addEventListener('dragover', e => { e.preventDefault(); dropZone.classList.add('drag-over'); });
dropZone.addEventListener('dragleave', () => dropZone.classList.remove('drag-over'));
dropZone.addEventListener('drop', e => {
  e.preventDefault();
  dropZone.classList.remove('drag-over');
  const file = e.dataTransfer.files[0];
  if (file) handleFile(file);
});
fileInput.addEventListener('change', () => { if (fileInput.files[0]) handleFile(fileInput.files[0]); });

async function handleFile(file) {
  clearError('upload-error');
  const body = new FormData();
  body.append('image', file);

  let resp;
  try {
    resp = await fetch('/api/v1/images', { method: 'POST', body });
  } catch {
    showError('upload-error', 'Network error — is the server running?');
    return;
  }

  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: 'Upload failed' }));
    showError('upload-error', err.error ?? 'Upload failed');
    return;
  }

  const img = await resp.json();
  imageId = img.id;

  const previewImg = $('preview-img');
  previewImg.src = URL.createObjectURL(file);
  previewImg.style.display = 'block';

  const meta = $('upload-meta');
  meta.innerHTML = '';
  [
    `${img.width} × ${img.height} px`,
    img.content_type,
    `${(img.file_size / 1024).toFixed(1)} KB`,
    img.original_filename,
  ].forEach(text => {
    const span = document.createElement('span');
    span.className = 'badge';
    span.textContent = text;
    meta.appendChild(span);
  });
  meta.classList.remove('hidden');

  $('btn-create-job').disabled = false;
  sectionJob.classList.remove('hidden');
  sectionJob.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

const opSelect = $('op-select');
opSelect.addEventListener('change', updateParamVisibility);

function updateParamVisibility() {
  ['resize', 'compress', 'convert'].forEach(op => {
    $(`params-${op}`).classList.toggle('visible', op === opSelect.value);
  });
}

$('btn-create-job').addEventListener('click', createJob);

async function createJob() {
  if (!imageId) return;
  clearError('job-error');

  const operation = opSelect.value;
  const parameters = buildParams(operation);

  let resp;
  try {
    resp = await fetch('/api/v1/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ image_id: imageId, operation, parameters }),
    });
  } catch {
    showError('job-error', 'Network error creating job');
    return;
  }

  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: 'Job creation failed' }));
    showError('job-error', err.error ?? 'Job creation failed');
    return;
  }

  const job = await resp.json();
  jobId = job.id;

  $('job-id-display').textContent = job.id;
  $('job-op-display').textContent = job.operation;
  updateStatusBadge('PENDING');
  $('result-section').classList.add('hidden');
  $('job-error-msg').style.display = 'none';

  sectionStatus.classList.remove('hidden');
  sectionStatus.scrollIntoView({ behavior: 'smooth', block: 'nearest' });

  startPolling();
}

function buildParams(operation) {
  switch (operation) {
    case 'resize': {
      const w = parseInt($('param-width').value, 10);
      const h = parseInt($('param-height').value, 10);
      const p = {};
      if (w > 0) p.width  = w;
      if (h > 0) p.height = h;
      return p;
    }
    case 'compress': {
      const q = parseInt($('param-quality').value, 10);
      return q > 0 ? { quality: q } : {};
    }
    case 'convert':
      return { format: $('param-format').value };
    default:
      return {};
  }
}

function startPolling() {
  if (pollTimer !== null) clearInterval(pollTimer);
  pollTimer = setInterval(pollJob, 2000);
}

function stopPolling() {
  if (pollTimer !== null) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

async function pollJob() {
  if (!jobId) return;
  let resp;
  try {
    resp = await fetch(`/api/v1/jobs/${jobId}`);
  } catch {
    return;
  }
  if (!resp.ok) return;

  const job = await resp.json();
  updateStatusBadge(job.status);

  if (job.status === 'COMPLETED') {
    stopPolling();
    showResult(job);
  } else if (job.status === 'FAILED') {
    stopPolling();
    const msg = $('job-error-msg');
    msg.textContent = 'Job failed: ' + (job.error_message ?? 'unknown error');
    msg.style.display = 'block';
  }
}

function updateStatusBadge(status) {
  const badge = $('job-status-badge');
  badge.textContent = status;
  badge.className = `status-badge status-${status}`;
}

async function showResult(job) {
  $('result-section').classList.remove('hidden');

  const resultUrl = `/api/v1/jobs/${job.id}/result`;
  let finalUrl = null;
  try {
    const r = await fetch(resultUrl, { method: 'GET', redirect: 'follow' });
    if (!r.ok) {
      const msg = $('job-error-msg');
      msg.textContent = `Could not load result (HTTP ${r.status})`;
      msg.style.display = 'block';
      return;
    }
    finalUrl = r.url;
  } catch {
    const msg = $('job-error-msg');
    msg.textContent = 'Network error loading result';
    msg.style.display = 'block';
    return;
  }

  const resultImg = $('result-img');
  resultImg.src   = finalUrl;
  resultImg.style.display = 'block';

  const dl = $('btn-download-result');
  dl.href = finalUrl;
  dl.download = `result_${job.id}.jpg`;
}

$('btn-new-job').addEventListener('click', () => {
  stopPolling();
  jobId = null;
  sectionStatus.classList.add('hidden');
  updateStatusBadge('PENDING');
  $('result-section').classList.add('hidden');
  sectionJob.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
});

$('btn-reset').addEventListener('click', () => {
  stopPolling();
  imageId = null;
  jobId   = null;
  fileInput.value = '';
  $('preview-img').style.display = 'none';
  $('upload-meta').classList.add('hidden');
  $('btn-create-job').disabled = true;
  clearError('upload-error');
  clearError('job-error');
  $('result-section').classList.add('hidden');
  sectionJob.classList.add('hidden');
  sectionStatus.classList.add('hidden');
  sectionUpload.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
});

function showError(id, msg) {
  const el = $(id);
  el.textContent = msg;
  el.style.display = 'block';
}

function clearError(id) {
  const el = $(id);
  el.textContent = '';
  el.style.display = 'none';
}
