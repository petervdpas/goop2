(() => {
  var core = window.Goop && window.Goop.core;
  if (!core) return;

  var qs = core.qs;
  var on = core.on;
  var escapeHtml = core.escapeHtml;
  var setHidden = core.setHidden;
  var toast = core.toast;
  var api = window.Goop.api;
  var gsel = window.Goop.select;

  var page = qs("#cluster-page");
  if (!page) return;

  var idleSection   = qs("#cl-idle");
  var hostSection   = qs("#cl-host");
  var workerSection = qs("#cl-worker");

  var createNameInput = qs("#cl-create-name");
  var createBtn       = qs("#cl-create-btn");

  var hostTitle   = qs("#cl-host-title");
  var hostGroupId = qs("#cl-host-group-id");
  var inviteBtn   = qs("#cl-invite-btn");
  var leaveHostBtn = qs("#cl-leave-host-btn");

  var statWorkers   = qs("#cl-stat-workers");
  var statPending   = qs("#cl-stat-pending");
  var statRunning   = qs("#cl-stat-running");
  var statCompleted = qs("#cl-stat-completed");
  var statFailed    = qs("#cl-stat-failed");

  var jobType     = qs("#cl-job-type");
  var jobPayload  = qs("#cl-job-payload");
  var jobPriority = qs("#cl-job-priority");
  var jobTimeout  = qs("#cl-job-timeout");
  var jobRetry    = qs("#cl-job-retry");
  var submitBtn   = qs("#cl-submit-btn");

  var workersListEl = qs("#cl-workers-list");
  var jobsListEl    = qs("#cl-jobs-list");

  var workerGroupId  = qs("#cl-worker-group-id");
  var leaveWorkerBtn = qs("#cl-leave-worker-btn");
  var binaryPath     = qs("#cl-binary-path");
  var binaryModeEl   = qs("#cl-binary-mode");
  var binaryBtn      = qs("#cl-binary-btn");
  var workerStatusEl = qs("#cl-worker-status");

  var _groupID = "";
  var _role = "none";
  var _pollTimer = null;

  function showSection(role) {
    setHidden(idleSection,   role !== "none");
    setHidden(hostSection,   role !== "host");
    setHidden(workerSection, role !== "worker");
  }

  function loadStatus() {
    api.cluster.status().then(function (data) {
      _role = data.role || "none";
      _groupID = data.group_id || "";
      showSection(_role);

      if (_role === "host") {
        hostGroupId.textContent = _groupID ? _groupID.substring(0, 12) + "\u2026" : "";
        if (data.stats) updateStats(data.stats);
        loadWorkers();
        loadJobs();
        startPolling();
      } else if (_role === "worker") {
        workerGroupId.textContent = _groupID ? _groupID.substring(0, 12) + "\u2026" : "";
        if (data.binary_path) binaryPath.value = data.binary_path;
        workerStatusEl.textContent = "Status: connected";
      } else {
        stopPolling();
      }
    }).catch(function () {
      showSection("none");
    });
  }

  function updateStats(s) {
    statWorkers.textContent   = s.workers   || 0;
    statPending.textContent   = s.pending   || 0;
    statRunning.textContent   = s.running   || 0;
    statCompleted.textContent = s.completed || 0;
    statFailed.textContent    = s.failed    || 0;
  }

  function loadWorkers() {
    api.cluster.workers().then(function (workers) {
      if (!workers || workers.length === 0) {
        workersListEl.innerHTML = '<p class="muted small">No workers connected.</p>';
        return;
      }
      var html = '<table class="cl-table"><thead><tr>' +
        '<th>Peer</th><th>Status</th><th>Binary</th><th>Mode</th><th>Capacity</th><th>Running</th>' +
        '</tr></thead><tbody>';
      workers.forEach(function (w) {
        var peerLabel = w.peer_id ? w.peer_id.substring(0, 10) + "\u2026" : "?";
        if (window.Goop.mq && window.Goop.mq.getPeerName) {
          var name = window.Goop.mq.getPeerName(w.peer_id);
          if (name) peerLabel = name;
        }
        html += '<tr>' +
          '<td>' + escapeHtml(peerLabel) + '</td>' +
          '<td><span class="cl-status-' + w.status + '">' + escapeHtml(w.status) + '</span></td>' +
          '<td>' + escapeHtml(w.binary_path || '-') + '</td>' +
          '<td>' + escapeHtml(w.binary_mode || '-') + '</td>' +
          '<td>' + (w.capacity || 1) + '</td>' +
          '<td>' + (w.running_jobs || 0) + '</td>' +
          '</tr>';
      });
      html += '</tbody></table>';
      workersListEl.innerHTML = html;
    });
  }

  function loadJobs() {
    api.cluster.jobs().then(function (jobs) {
      if (!jobs || jobs.length === 0) {
        jobsListEl.innerHTML = '<p class="muted small">No jobs submitted.</p>';
        return;
      }
      var html = '<table class="cl-table"><thead><tr>' +
        '<th>ID</th><th>Type</th><th>Status</th><th>Progress</th><th>Worker</th><th>Retries</th><th>Actions</th>' +
        '</tr></thead><tbody>';
      jobs.forEach(function (j) {
        var job = j.job || {};
        var shortId = (job.id || "?").substring(0, 8);
        var workerLabel = j.worker_id ? j.worker_id.substring(0, 8) + "\u2026" : "-";
        var progressHtml = "";
        if (j.progress > 0) {
          progressHtml = j.progress + '%' +
            '<div class="cl-progress"><div class="cl-progress-fill" style="width:' + Math.min(j.progress, 100) + '%"></div></div>';
          if (j.progress_msg) progressHtml += ' <span class="muted small">' + escapeHtml(j.progress_msg) + '</span>';
        } else {
          progressHtml = "-";
        }
        var canCancel = j.status === "pending" || j.status === "assigned" || j.status === "running";
        var actionsHtml = canCancel
          ? '<button class="btn btn-danger btn-small cl-cancel-btn" data-job-id="' + escapeHtml(job.id) + '">Cancel</button>'
          : '';
        if (j.error) {
          actionsHtml += ' <span class="cl-status-failed" title="' + escapeHtml(j.error) + '">err</span>';
        }
        html += '<tr>' +
          '<td title="' + escapeHtml(job.id || '') + '">' + escapeHtml(shortId) + '</td>' +
          '<td>' + escapeHtml(job.type || '?') + '</td>' +
          '<td><span class="cl-status-' + j.status + '">' + escapeHtml(j.status) + '</span></td>' +
          '<td>' + progressHtml + '</td>' +
          '<td>' + escapeHtml(workerLabel) + '</td>' +
          '<td>' + (j.retries || 0) + '/' + (job.max_retry || 0) + '</td>' +
          '<td>' + actionsHtml + '</td>' +
          '</tr>';
      });
      html += '</tbody></table>';
      jobsListEl.innerHTML = html;

      jobsListEl.querySelectorAll(".cl-cancel-btn").forEach(function (btn) {
        on(btn, "click", function () {
          var jobId = btn.getAttribute("data-job-id");
          api.cluster.cancel({ job_id: jobId }).then(function () {
            toast("Job cancelled");
            loadJobs();
          }).catch(function (err) { toast("Cancel failed: " + err.message, true); });
        });
      });
    });
  }

  function startPolling() {
    stopPolling();
    _pollTimer = setInterval(function () {
      if (_role !== "host") { stopPolling(); return; }
      api.cluster.stats().then(updateStats).catch(function () {});
      loadWorkers();
      loadJobs();
    }, 3000);
  }

  function stopPolling() {
    if (_pollTimer) { clearInterval(_pollTimer); _pollTimer = null; }
  }

  on(createBtn, "click", function () {
    var name = createNameInput.value.trim() || "Cluster";
    api.cluster.create({ name: name }).then(function (data) {
      toast("Cluster created");
      _groupID = data.group_id;
      loadStatus();
    }).catch(function (err) { toast("Failed: " + err.message, true); });
  });

  on(leaveHostBtn, "click", function () {
    if (!window.Goop.dialogs) { doLeave(); return; }
    window.Goop.dialogs.confirm("Close this cluster? All workers will be disconnected.", "Close Cluster").then(function (ok) {
      if (ok) doLeave();
    });
  });

  on(leaveWorkerBtn, "click", function () {
    doLeave();
  });

  function doLeave() {
    api.cluster.leave().then(function () {
      toast("Left cluster");
      _role = "none";
      _groupID = "";
      showSection("none");
      stopPolling();
    }).catch(function (err) { toast("Failed: " + err.message, true); });
  }

  on(inviteBtn, "click", function (e) {
    e.stopPropagation();
    if (_groupID && window.Goop && window.Goop.groups) {
      window.Goop.groups.showInvitePopup(_groupID, inviteBtn);
    }
  });

  on(submitBtn, "click", function () {
    var type = jobType.value.trim();
    if (!type) { toast("Job type is required", true); return; }
    var payload = {};
    var raw = jobPayload.value.trim();
    if (raw) {
      try { payload = JSON.parse(raw); } catch (_) {
        toast("Invalid JSON payload", true);
        return;
      }
    }
    api.cluster.submit({
      type:      type,
      payload:   payload,
      priority:  parseInt(jobPriority.value, 10) || 0,
      timeout_s: parseInt(jobTimeout.value, 10) || 0,
      max_retry: parseInt(jobRetry.value, 10) || 0,
    }).then(function () {
      toast("Job submitted");
      jobType.value = "";
      jobPayload.value = "";
      loadJobs();
    }).catch(function (err) { toast("Submit failed: " + err.message, true); });
  });

  if (binaryModeEl) gsel.init(binaryModeEl);

  on(binaryBtn, "click", function () {
    var path = binaryPath.value.trim();
    if (!path) { toast("Binary path is required", true); return; }
    var mode = gsel.val(binaryModeEl) || "oneshot";
    api.cluster.binary({ path: path, mode: mode }).then(function () {
      toast("Binary set");
      workerStatusEl.textContent = "Binary: " + path + " (" + mode + ")";
    }).catch(function (err) { toast("Failed: " + err.message, true); });
  });

  loadStatus();
})();
