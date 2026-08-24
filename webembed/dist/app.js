// LeafWash 联检控制台：轮询 Go 后端健康状态，创建联检任务并按需加载任务详情。
async function probeHealth() {
  const statusEl = document.getElementById('backend-status');
  const detailEl = document.getElementById('backend-detail');
  try {
    const res = await fetch('/healthz');
    const body = await res.json();
    statusEl.textContent = body.status;
    detailEl.textContent = 'GET /healthz -> ' + JSON.stringify(body);
  } catch (err) {
    statusEl.textContent = 'unreachable';
    detailEl.textContent = '无法连接后端: ' + err.message;
  }
}

async function lockTask() {
  const resultEl = document.getElementById('lock-result');
  const payload = {
    task_id: document.getElementById('lock-task-id').value.trim(),
    base_lot_id: document.getElementById('lock-base-lot').value.trim(),
    seal_id: document.getElementById('lock-seal').value.trim(),
    precool_lot: 'PC-001',
    cut_line_id: 'CUT-3',
    wash_tank_id: document.getElementById('lock-tank').value.trim(),
    formula_id: document.getElementById('lock-formula').value.trim(),
    formula_revision: 3,
    sample_times: [0, 300, 600],
    blind_codes: ['BLIND-01', 'BLIND-02', 'BLIND-03'],
    atp_points: ['ATP-1', 'ATP-2', 'ATP-3'],
    plate_wells: ['WELL-A1', 'WELL-A2', 'WELL-B1'],
    drain_slots: ['DRAIN-1', 'DRAIN-2'],
    reviewers: ['P-2001', 'P-2002'],
  };
  try {
    const res = await fetch('/api/tasks/lock', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    const body = await res.json();
    resultEl.textContent = res.status + ' ' + JSON.stringify(body, null, 2);
    document.getElementById('task-id').value = payload.task_id;
    loadTask();
  } catch (err) {
    resultEl.textContent = '锁定失败: ' + err.message;
  }
}

async function loadTask() {
  const id = document.getElementById('task-id').value.trim();
  const detailEl = document.getElementById('task-detail');
  const res = await fetch('/api/tasks/' + encodeURIComponent(id));
  if (!res.ok) {
    detailEl.textContent = res.status + ' ' + (await res.text());
    return;
  }
  const body = await res.json();
  detailEl.textContent = JSON.stringify(body, null, 2);
}

document.getElementById('load-task').addEventListener('click', loadTask);
document.getElementById('do-lock').addEventListener('click', lockTask);
probeHealth();
setInterval(probeHealth, 5000);
loadTask();
