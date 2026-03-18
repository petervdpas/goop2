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

  var groupsListEl  = qs("#cl-groups-list");
  var createNameInput = qs("#cl-create-name");
  var createMaxInput  = qs("#cl-create-max");
  var createBtn     = qs("#cl-create-btn");
  var refreshBtn    = qs("#cl-refresh-btn");

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
  var jobMode     = qs("#cl-job-mode");
  var jobFieldsEl = qs("#cl-job-fields");
  var jobPriority = qs("#cl-job-priority");
  var jobTimeout  = qs("#cl-job-timeout");
  var jobRetry    = qs("#cl-job-retry");
  var submitBtn   = qs("#cl-submit-btn");

  var workersListEl = qs("#cl-workers-list");
  var jobsListEl    = qs("#cl-jobs-list");

  var workerGroupId  = qs("#cl-worker-group-id");
  var leaveWorkerBtn = qs("#cl-leave-worker-btn");
  var binaryModeEl   = qs("#cl-binary-mode");
  var binaryBtn      = qs("#cl-binary-btn");
  var workerStatusEl = qs("#cl-worker-status");

  var _groupID = "";
  var _role = "none";
  var _pollTimer = null;
  var _clusterGroups = [];

  function showSection(role) {
    setHidden(idleSection,   role !== "none");
    setHidden(hostSection,   role !== "host");
    setHidden(workerSection, role !== "worker");
    var inactive = role !== "none";
    createBtn.disabled = inactive;
    createNameInput.disabled = inactive;
    if (createMaxInput) createMaxInput.disabled = inactive;
  }

  function loadClusterGroups() {
    Promise.all([
      api.groups.list().catch(function () { return []; }),
      api.groups.subscriptions().catch(function () { return { subscriptions: [] }; })
    ]).then(function (results) {
      var hosted = (results[0] || []).filter(function (g) { return g.app_type === "cluster"; });
      var subsData = results[1] || {};
      var subs = (subsData.subscriptions || []).filter(function (s) { return s.app_type === "cluster"; });

      _clusterGroups = [];
      hosted.forEach(function (g) {
        _clusterGroups.push({ id: g.id, name: g.name, source: "hosted", members: g.members || 0, maxMembers: g.max_members || 0 });
      });
      subs.forEach(function (s) {
        _clusterGroups.push({ id: s.group_id, name: s.group_name || s.group_id, source: "joined", hostPeerID: s.host_peer_id || "" });
      });

      renderClusterGroups();
    });
  }

  function renderClusterGroups() {
    if (_clusterGroups.length === 0) {
      groupsListEl.innerHTML = '<p class="empty-state">No cluster groups.</p>';
      return;
    }
    var html = "";
    _clusterGroups.forEach(function (g) {
      var isActive = g.id === _groupID;
      var shortId = g.id.substring(0, 10) + "\u2026";
      var badge = g.source === "hosted" ? "hosted" : "joined";
      var actionBtn = "";

      if (isActive) {
        actionBtn = '<span class="badge badge-' + (_role === "host" ? "host" : "worker") + '">' +
          escapeHtml(_role.toUpperCase()) + '</span>';
      } else if (_role === "none") {
        if (g.source === "hosted") {
          actionBtn = '<button class="groups-action-btn groups-btn-primary cl-host-btn" data-group-id="' +
            escapeHtml(g.id) + '">Host</button>';
        } else {
          actionBtn = '<button class="groups-action-btn groups-btn-primary cl-join-btn" data-group-id="' +
            escapeHtml(g.id) + '" data-host-peer="' + escapeHtml(g.hostPeerID || '') + '">Join</button>';
        }
      }

      html += '<div class="cl-group-item' + (isActive ? ' cl-active' : '') + '" data-group-id="' + escapeHtml(g.id) + '">' +
        '<div class="cl-group-info">' +
          '<span class="cl-group-name">' + escapeHtml(g.name) + '</span>' +
          '<span class="cl-group-id">' + escapeHtml(shortId) + ' &middot; ' + badge + '</span>' +
        '</div>' +
        '<div class="cl-group-actions">' + actionBtn + '</div>' +
      '</div>';
    });
    groupsListEl.innerHTML = html;

    groupsListEl.querySelectorAll(".cl-host-btn").forEach(function (btn) {
      on(btn, "click", function (e) {
        e.stopPropagation();
        var gid = btn.getAttribute("data-group-id");
        api.cluster.create({ name: "", group_id: gid }).catch(function () {
          return api.cluster.join({ group_id: gid, host_peer_id: "" });
        }).then(function () {
          loadStatus();
          loadClusterGroups();
        }).catch(function (err) { toast("Failed: " + err.message, true); });
      });
    });

    groupsListEl.querySelectorAll(".cl-join-btn").forEach(function (btn) {
      on(btn, "click", function (e) {
        e.stopPropagation();
        var gid = btn.getAttribute("data-group-id");
        var hostPeer = btn.getAttribute("data-host-peer") || "";
        api.cluster.join({ group_id: gid, host_peer_id: hostPeer }).then(function () {
          toast("Joined cluster");
          loadStatus();
          loadClusterGroups();
        }).catch(function (err) { toast("Failed: " + err.message, true); });
      });
    });
  }

  function loadStatus() {
    api.cluster.status().then(function (data) {
      _role = data.role || "none";
      _groupID = data.group_id || "";
      showSection(_role);

      if (_role === "host") {
        var grp = _clusterGroups.find(function (g) { return g.id === _groupID; });
        hostTitle.textContent = (grp && grp.name) || "Cluster";
        hostGroupId.textContent = _groupID ? _groupID.substring(0, 12) + "\u2026" : "";
        var maxEl = qs("#cl-max-workers");
        if (maxEl && grp) maxEl.value = grp.maxMembers || 0;
        if (data.stats) updateStats(data.stats);
        loadWorkers();
        loadJobs();
        startPolling();
      } else if (_role === "worker") {
        workerGroupId.textContent = _groupID ? _groupID.substring(0, 12) + "\u2026" : "";
        if (data.binary_path && binaryPicker) binaryPicker.setValue(data.binary_path);
        workerStatusEl.textContent = "Status: connected";
      } else {
        stopPolling();
      }

      renderClusterGroups();
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
      var html = '<table class="data-table"><thead><tr>' +
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
      var html = '<table class="data-table"><thead><tr>' +
        '<th>ID</th><th>Type</th><th>Status</th><th>Progress</th><th>Worker</th><th>Retries</th><th>Actions</th>' +
        '</tr></thead><tbody>';
      jobs.forEach(function (j) {
        var job = j.job || {};
        var shortId = (job.id || "?").substring(0, 8);
        var workerLabel = j.worker_id ? j.worker_id.substring(0, 8) + "\u2026" : "-";
        var progressHtml = "";
        if (j.progress > 0) {
          progressHtml = j.progress + '%' +
            '<div class="progress-bar"><div class="progress-fill" style="width:' + Math.min(j.progress, 100) + '%"></div></div>';
          if (j.progress_msg) progressHtml += ' <span class="muted small">' + escapeHtml(j.progress_msg) + '</span>';
        } else {
          progressHtml = "-";
        }
        var canCancel = j.status === "pending" || j.status === "assigned" || j.status === "running";
        var canDelete = j.status === "cancelled" || j.status === "completed" || j.status === "failed";
        var actionsHtml = '';
        if (canCancel) {
          actionsHtml += '<button class="btn btn-danger btn-small cl-cancel-btn" data-job-id="' + escapeHtml(job.id) + '">Cancel</button>';
        }
        if (canDelete) {
          actionsHtml += '<button class="btn btn-small cl-delete-btn" data-job-id="' + escapeHtml(job.id) + '">Delete</button>';
        }
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

      jobsListEl.querySelectorAll(".cl-delete-btn").forEach(function (btn) {
        on(btn, "click", function () {
          var jobId = btn.getAttribute("data-job-id");
          api.cluster.delete({ job_id: jobId }).then(function () {
            loadJobs();
          }).catch(function (err) { toast("Delete failed: " + err.message, true); });
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
    var maxMembers = parseInt((createMaxInput && createMaxInput.value) || "0", 10) || 0;
    api.cluster.create({ name: name, max_members: maxMembers }).then(function (data) {
      toast("Cluster created");
      createNameInput.value = "";
      if (createMaxInput) createMaxInput.value = "0";
      _groupID = data.group_id;
      loadClusterGroups();
      loadStatus();
    }).catch(function (err) { toast("Failed: " + err.message, true); });
  });

  on(refreshBtn, "click", function () {
    loadClusterGroups();
    loadStatus();
  });

  var maxSetBtn = qs("#cl-max-set-btn");
  on(maxSetBtn, "click", function () {
    var maxEl = qs("#cl-max-workers");
    var val = parseInt(maxEl && maxEl.value || "0", 10) || 0;
    if (!_groupID) return;
    api.groups.setMaxMembers({ group_id: _groupID, max_members: val }).then(function () {
      toast("Max workers updated");
      loadClusterGroups();
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
      loadClusterGroups();
    }).catch(function (err) { toast("Failed: " + err.message, true); });
  }

  on(inviteBtn, "click", function (e) {
    e.stopPropagation();
    if (_groupID && window.Goop && window.Goop.groups) {
      window.Goop.groups.showInvitePopup(_groupID, inviteBtn);
    }
  });

  var _jobTypes = [];
  var jobPayload = qs("#cl-job-payload");
  var payloadInfo = qs("#cl-payload-info");

  function loadJobTypes() {
    api.cluster.types().then(function (types) {
      _jobTypes = types || [];
      var opts = _jobTypes.map(function (t) {
        return { value: t.name, label: t.name + ' \u2014 ' + t.description };
      });
      gsel.setOpts(jobType, { options: opts }, "");
    });
  }

  gsel.init(jobMode);
  gsel.init(jobType, function (val) {
    var t = _jobTypes.find(function (t) { return t.name === val; });
    if (t && t.template) jobPayload.value = t.template;
    if (payloadInfo) {
      var helpText = payloadInfo.querySelector('.panel-info-text');
      if (t && t.help) {
        if (helpText) helpText.textContent = t.help;
        setHidden(payloadInfo, false);
      } else {
        setHidden(payloadInfo, true);
      }
    }
    validatePayload();
  });

  function validatePayload() {
    var v = jobPayload.value.trim();
    if (!v) { jobPayload.classList.remove('json-valid', 'json-invalid'); return; }
    var r = core.validateJSON(v);
    jobPayload.classList.toggle('json-valid', r.ok);
    jobPayload.classList.toggle('json-invalid', !r.ok);
  }

  on(jobPayload, 'blur', validatePayload);
  on(jobPayload, 'input', function () {
    jobPayload.classList.remove('json-valid', 'json-invalid');
  });

  on(submitBtn, "click", function () {
    var type = gsel.val(jobType);
    if (!type) { toast("Select a job type", true); return; }

    var p = core.validateJSON(jobPayload.value);
    if (!p.ok) { toast("Payload: " + p.error, true); jobPayload.focus(); return; }

    api.cluster.submit({
      type:      type,
      mode:      gsel.val(jobMode) || 'oneshot',
      payload:   p.value || {},
      priority:  parseInt(jobPriority.value, 10) || 0,
      timeout_s: parseInt(jobTimeout.value, 10) || 0,
      max_retry: parseInt(jobRetry.value, 10) || 0,
    }).then(function () {
      toast("Job submitted");
      gsel.setVal(jobType, "");
      jobPayload.value = "";
      jobPayload.classList.remove('json-valid', 'json-invalid');
      loadJobs();
    }).catch(function (err) { toast("Submit failed: " + err.message, true); });
  });

  loadJobTypes();

  if (binaryModeEl) gsel.init(binaryModeEl);

  var binaryPicker = window.Goop.filepicker && window.Goop.filepicker.init(
    qs(".filepicker", workerSection),
    { title: "Select Worker Binary" }
  );

  on(binaryBtn, "click", function () {
    var path = binaryPicker ? binaryPicker.value() : "";
    if (!path) { toast("Binary path is required", true); return; }
    var mode = gsel.val(binaryModeEl) || "oneshot";
    api.cluster.binary({ path: path, mode: mode }).then(function () {
      toast("Binary set");
      workerStatusEl.textContent = "Binary: " + path + " (" + mode + ")";
    }).catch(function (err) { toast("Failed: " + err.message, true); });
  });

  loadClusterGroups();
  loadStatus();
})();
