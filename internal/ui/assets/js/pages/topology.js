// Topology graph: canvas-drawn network topology on the Logs page.
(() => {
  var canvas = document.getElementById('topology-canvas');
  if (!canvas) return;

  var _hover = null;
  var _data = null;
  var _timer = null;
  var _pulse = 0;
  var _pulseTimer = null;

  // Zoom & pan state.
  var _zoom = 1;
  var _panX = 0, _panY = 0;
  var _dragging = false;
  var _dragStart = null;
  var _panStart = null;

  // Node dragging.
  var _dragNode = null;    // node being dragged (from canvas._nodes)
  var _manualPos = {};     // peer id → {x, y} — manually placed nodes

  window._topologyStart = function() {
    _topologyStop();
    _zoom = 1; _panX = 0; _panY = 0; _manualPos = {};
    fetchData();
    _timer = setInterval(fetchData, 10000);
    _pulse = 0;
    _pulseTimer = setInterval(function() {
      _pulse = (_pulse + 1) % 60;
      if (canvas.style.display !== 'none' && _data) render();
    }, 50);
  };
  window._topologyStop = function() {
    if (_timer) { clearInterval(_timer); _timer = null; }
    if (_pulseTimer) { clearInterval(_pulseTimer); _pulseTimer = null; }
  };

  function fetchData() {
    fetch('/api/topology').then(function(r) { return r.json(); }).then(function(data) {
      _data = data;
      render();
    }).catch(function(err) {
      _data = null;
      var dpr = window.devicePixelRatio || 1;
      var rect = canvas.getBoundingClientRect();
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      var ctx = canvas.getContext('2d');
      ctx.scale(dpr, dpr);
      var cs = getComputedStyle(document.documentElement);
      ctx.fillStyle = cs.getPropertyValue('--bg').trim() || '#0f1115';
      ctx.fillRect(0, 0, rect.width, rect.height);
      ctx.fillStyle = cs.getPropertyValue('--muted').trim() || '#9aa3b2';
      ctx.font = '14px system-ui,sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText('Topology unavailable: ' + err, rect.width / 2, rect.height / 2);
    });
  }

  function render() {
    if (!_data) return;
    var data = _data;
    var dpr = window.devicePixelRatio || 1;
    var rect = canvas.getBoundingClientRect();
    canvas.width = rect.width * dpr;
    canvas.height = rect.height * dpr;
    var ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    var W = rect.width, H = rect.height;

    var cs = getComputedStyle(document.documentElement);
    var colBg     = cs.getPropertyValue('--bg').trim() || '#0f1115';
    var colText   = cs.getPropertyValue('--text').trim() || '#e6e9ef';
    var colMuted  = cs.getPropertyValue('--muted').trim() || '#9aa3b2';
    var colAccent = cs.getPropertyValue('--accent').trim() || '#7aa2ff';
    var colPanel  = cs.getPropertyValue('--panel').trim() || '#151924';
    var colDirect = '#4ade80';
    var colRelay  = colAccent;
    var colNone   = '#555b6e';

    ctx.fillStyle = colBg;
    ctx.fillRect(0, 0, W, H);

    // Apply zoom & pan transform. Everything after this draws in world coords.
    ctx.save();
    ctx.translate(W / 2 + _panX, H / 2 + _panY);
    ctx.scale(_zoom, _zoom);
    ctx.translate(-W / 2, -H / 2);

    var hasRelay = !!data.relay;
    var allPeers = data.peers || [];
    var directPeers = [], relayPeers = [], offlinePeers = [];
    allPeers.forEach(function(p) {
      if (p.connection === 'direct') directPeers.push(p);
      else if (p.connection === 'relay') relayPeers.push(p);
      else offlinePeers.push(p);
    });

    var onlineCount = directPeers.length + relayPeers.length;
    var totalCount = allPeers.length;

    // ── Layout ──────────────────────────────────────────────────────────
    // Self sits slightly above center. Relay above-left. All connected
    // peers radiate from self in a full circle. Offline peers are smaller,
    // clustered in the lower portion.

    var selfX = W / 2;
    var selfY = hasRelay ? H * 0.52 : H * 0.45;
    var selfR = 32;
    var nodeR = 24;
    var orbitR = Math.min(W, H) * 0.30;
    // Ensure orbit doesn't push nodes off-screen.
    orbitR = Math.min(orbitR, selfX - nodeR - 40, selfY - nodeR - 60,
                      W - selfX - nodeR - 40, H - selfY - nodeR - 50);
    if (orbitR < 80) orbitR = 80;

    // Relay position: upper-left area with breathing room.
    var relayX = W * 0.28;
    var relayY = H * 0.18;
    var relayR = 20;

    // Place all connected peers in a circle around self.
    var connectedPeers = directPeers.concat(relayPeers);
    var connectedPos = layoutCircle(connectedPeers.length, selfX, selfY, orbitR);

    // Apply manual positions (from drag).
    if (_manualPos['self']) { selfX = _manualPos['self'].x; selfY = _manualPos['self'].y; }
    if (hasRelay && _manualPos[data.relay.id]) { relayX = _manualPos[data.relay.id].x; relayY = _manualPos[data.relay.id].y; }
    connectedPeers.forEach(function(p, i) {
      if (_manualPos[p.id]) { connectedPos[i] = _manualPos[p.id]; }
    });

    var nodes = [];

    // ── Subtle grid / ring guides ───────────────────────────────────────
    ctx.save();
    ctx.strokeStyle = colMuted;
    ctx.globalAlpha = 0.06;
    ctx.lineWidth = 1;
    ctx.setLineDash([2, 6]);
    ctx.beginPath();
    ctx.arc(selfX, selfY, orbitR, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();

    // ── Edges ───────────────────────────────────────────────────────────
    // Self → relay.
    if (hasRelay) {
      var relayOk = data.self.has_circuit;
      drawCurvedEdge(ctx, selfX, selfY, relayX, relayY,
        relayOk ? colRelay : colNone, !relayOk, 1.8);
    }

    // Self → connected peers.
    connectedPeers.forEach(function(p, i) {
      var pos = connectedPos[i];
      var col = p.connection === 'direct' ? colDirect : colRelay;
      var dashed = p.connection === 'relay';
      drawCurvedEdge(ctx, selfX, selfY, pos.x, pos.y, col, dashed, 1.5);
    });

    // ── Relay → relay peers (secondary edges) ───────────────────────────
    if (hasRelay) {
      relayPeers.forEach(function(p) {
        // Find this peer's position.
        var idx = connectedPeers.indexOf(p);
        if (idx < 0) return;
        var pos = connectedPos[idx];
        drawCurvedEdge(ctx, relayX, relayY, pos.x, pos.y, colRelay, true, 0.7);
      });
    }

    // ── Draw relay node ─────────────────────────────────────────────────
    if (hasRelay) {
      var relayOk = data.self.has_circuit;
      var rc = relayOk ? colRelay : colNone;
      drawGlowNode(ctx, relayX, relayY, relayR, rc, 'diamond');
      drawNodeLabel(ctx, data.relay.label || 'Relay', relayX, relayY + relayR + 12, colText, 11);
      if (!relayOk) {
        drawNodeLabel(ctx, 'no circuit', relayX, relayY + relayR + 26, colNone, 9);
      }
      nodes.push({ x: relayX, y: relayY, r: relayR, peer: data.relay, type: 'relay' });
    }

    // ── Draw connected peers ────────────────────────────────────────────
    connectedPeers.forEach(function(p, i) {
      var pos = connectedPos[i];
      var col = p.connection === 'direct' ? colDirect : colRelay;
      drawGlowNode(ctx, pos.x, pos.y, nodeR, col, 'circle');
      // Short label inside or below.
      var shortLabel = truncLabel(p.label, 10);
      drawNodeLabel(ctx, shortLabel, pos.x, pos.y + nodeR + 10, colText, 11);
      // Connection type badge.
      var badge = p.connection === 'direct' ? (isPrivateAddr(p.addr) ? 'LAN' : 'direct') : 'relay';
      drawBadge(ctx, pos.x, pos.y - nodeR - 6, badge,
        p.connection === 'direct' ? colDirect : colRelay, colBg);
      nodes.push({ x: pos.x, y: pos.y, r: nodeR, peer: p, type: p.connection });
    });

    // ── Self node (pulsing) ─────────────────────────────────────────────
    var pulseAlpha = 0.12 + 0.08 * Math.sin(_pulse * Math.PI / 30);
    var pulseR = selfR + 6 + 3 * Math.sin(_pulse * Math.PI / 30);

    // Outer pulse ring.
    ctx.save();
    ctx.beginPath();
    ctx.arc(selfX, selfY, pulseR, 0, Math.PI * 2);
    ctx.strokeStyle = colAccent;
    ctx.globalAlpha = pulseAlpha;
    ctx.lineWidth = 2;
    ctx.stroke();
    ctx.restore();

    drawGlowNode(ctx, selfX, selfY, selfR, colAccent, 'circle');

    // "ME" text inside.
    ctx.save();
    ctx.fillStyle = colBg;
    ctx.font = 'bold 14px system-ui,sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('ME', selfX, selfY);
    ctx.restore();

    drawNodeLabel(ctx, data.self.label, selfX, selfY + selfR + 12, colText, 12);
    nodes.push({ x: selfX, y: selfY, r: selfR, peer: data.self, type: 'self' });

    // Restore from zoom/pan transform — HUD elements draw in screen coords.
    ctx.restore();

    // ── Stats bar (top-right) ───────────────────────────────────────────
    ctx.save();
    ctx.font = '11px system-ui,sans-serif';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'top';
    ctx.fillStyle = colMuted;
    var statsX = W - 16, statsY = 12;
    ctx.fillText(onlineCount + ' online / ' + totalCount + ' known', statsX, statsY);
    if (hasRelay) {
      ctx.fillStyle = data.self.has_circuit ? colRelay : colNone;
      ctx.fillText('circuit: ' + (data.self.has_circuit ? 'active' : 'lost'), statsX, statsY + 16);
    }
    ctx.restore();

    // ── Zoom indicator (bottom-right) ──────────────────────────────────
    if (_zoom !== 1 || _panX !== 0 || _panY !== 0) {
      ctx.save();
      ctx.font = '10px system-ui,sans-serif';
      ctx.textAlign = 'right';
      ctx.textBaseline = 'bottom';
      ctx.fillStyle = colMuted;
      ctx.globalAlpha = 0.6;
      ctx.fillText(Math.round(_zoom * 100) + '%  (dbl-click to reset)', W - 16, H - 12);
      ctx.restore();
    }

    // ── Legend (bottom-left) ────────────────────────────────────────────
    var lx = 16, ly = H - 44;
    drawLegend(ctx, lx, ly, colDirect, false, 'Direct', colText);
    drawLegend(ctx, lx, ly + 18, colRelay, true, 'Relay', colText);

    // ── Hover tooltip (screen-space) ────────────────────────────────────
    if (_hover && !_dragNode) {
      var hit = hitNode(_hover.x, _hover.y);
      if (hit) {
        var lines = [hit.peer.label || hit.peer.id];
        if (hit.peer.id) lines.push(hit.peer.id.slice(0, 16) + '...');
        if (hit.peer.addr) lines.push(hit.peer.addr);
        if (hit.peer.age && hit.peer.age !== '0s') lines.push('uptime: ' + hit.peer.age);
        if (hit.peer.streams) lines.push('streams: ' + hit.peer.streams);
        if (hit.peer.connection) lines.push('via: ' + hit.peer.connection);
        drawTooltip(ctx, _hover.x, _hover.y - 16, lines, colPanel, colText);
      }
    }

    canvas._nodes = nodes;
  }

  // ── Layout ──────────────────────────────────────────────────────────────
  // Distribute N items evenly around a full circle, starting from top.
  function layoutCircle(count, cx, cy, r) {
    var out = [];
    if (count === 0) return out;
    var startAngle = -Math.PI / 2; // top
    for (var i = 0; i < count; i++) {
      var a = startAngle + (2 * Math.PI * i) / count;
      out.push({ x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) });
    }
    return out;
  }

  function layoutArc(count, cx, cy, r, a1, a2) {
    var out = [];
    if (count === 0) return out;
    if (count === 1) {
      var a = (a1 + a2) / 2;
      out.push({ x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) });
      return out;
    }
    for (var i = 0; i < count; i++) {
      var a = a1 + (a2 - a1) * i / (count - 1);
      out.push({ x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) });
    }
    return out;
  }

  // Check if a multiaddr points to a private/LAN IP.
  function isPrivateAddr(addr) {
    if (!addr) return false;
    // Extract IP from multiaddr like /ip4/192.168.1.42/tcp/4001
    var m = addr.match(/\/ip4\/([\d.]+)/);
    if (!m) return false;
    var ip = m[1];
    // RFC 1918 + loopback
    return ip.startsWith('10.') ||
           ip.startsWith('192.168.') ||
           ip.startsWith('127.') ||
           /^172\.(1[6-9]|2\d|3[01])\./.test(ip);
  }

  function truncLabel(s, max) {
    if (!s) return '';
    return s.length > max ? s.slice(0, max - 1) + '\u2026' : s;
  }

  // ── Drawing primitives ──────────────────────────────────────────────────
  function drawCurvedEdge(ctx, x1, y1, x2, y2, color, dashed, width) {
    ctx.save();
    ctx.strokeStyle = color;
    ctx.lineWidth = width || 1.5;
    ctx.globalAlpha = 0.4;
    if (dashed) ctx.setLineDash([6, 4]);
    // Slight curve via control point offset perpendicular to the line.
    var mx = (x1 + x2) / 2, my = (y1 + y2) / 2;
    var dx = x2 - x1, dy = y2 - y1;
    var len = Math.sqrt(dx * dx + dy * dy) || 1;
    var off = len * 0.08; // curve amount
    var cpx = mx + (-dy / len) * off;
    var cpy = my + (dx / len) * off;
    ctx.beginPath();
    ctx.moveTo(x1, y1);
    ctx.quadraticCurveTo(cpx, cpy, x2, y2);
    ctx.stroke();
    ctx.restore();
  }

  function drawGlowNode(ctx, x, y, r, color, shape) {
    ctx.save();

    // Outer glow.
    var grad = ctx.createRadialGradient(x, y, r * 0.5, x, y, r * 2.2);
    grad.addColorStop(0, color);
    grad.addColorStop(1, 'transparent');
    ctx.globalAlpha = 0.08;
    ctx.fillStyle = grad;
    ctx.fillRect(x - r * 2.5, y - r * 2.5, r * 5, r * 5);

    // Fill.
    ctx.globalAlpha = 0.2;
    ctx.fillStyle = color;
    if (shape === 'diamond') {
      ctx.beginPath();
      ctx.moveTo(x, y - r);
      ctx.lineTo(x + r, y);
      ctx.lineTo(x, y + r);
      ctx.lineTo(x - r, y);
      ctx.closePath();
      ctx.fill();
      ctx.globalAlpha = 1;
      ctx.strokeStyle = color;
      ctx.lineWidth = 2;
      ctx.stroke();
    } else {
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 1;
      ctx.strokeStyle = color;
      ctx.lineWidth = 2;
      ctx.stroke();
    }

    ctx.restore();
  }

  function drawNodeLabel(ctx, text, x, y, color, size) {
    if (!text) return;
    ctx.save();
    ctx.fillStyle = color;
    ctx.font = (size || 11) + 'px system-ui,sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'top';
    ctx.fillText(text, x, y);
    ctx.restore();
  }

  function drawBadge(ctx, x, y, text, color, bgColor) {
    ctx.save();
    ctx.font = '9px system-ui,sans-serif';
    var w = ctx.measureText(text).width + 8;
    var h = 14;
    ctx.fillStyle = color;
    ctx.globalAlpha = 0.18;
    ctx.beginPath();
    ctx.roundRect(x - w / 2, y - h / 2, w, h, 3);
    ctx.fill();
    ctx.globalAlpha = 1;
    ctx.fillStyle = color;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(text, x, y);
    ctx.restore();
  }

  function drawLegend(ctx, x, y, color, dashed, label, textColor) {
    ctx.save();
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.5;
    ctx.globalAlpha = 0.6;
    if (dashed) ctx.setLineDash([4, 3]);
    ctx.beginPath();
    ctx.moveTo(x, y + 5);
    ctx.lineTo(x + 20, y + 5);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.globalAlpha = 1;
    ctx.beginPath();
    ctx.arc(x + 20, y + 5, 3, 0, Math.PI * 2);
    ctx.fillStyle = color;
    ctx.fill();
    ctx.fillStyle = textColor;
    ctx.font = '10px system-ui,sans-serif';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    ctx.fillText(label, x + 28, y + 5);
    ctx.restore();
  }

  function drawTooltip(ctx, x, y, lines, bgColor, textColor) {
    ctx.save();
    ctx.font = '11px monospace';
    var pad = 10, lineH = 16, maxW = 0;
    lines.forEach(function(l) {
      var w = ctx.measureText(l).width;
      if (w > maxW) maxW = w;
    });
    var tw = maxW + pad * 2;
    var th = lines.length * lineH + pad * 2;
    var tx = x - tw / 2;
    var ty = y - th;
    var cw = canvas.getBoundingClientRect().width;
    var ch = canvas.getBoundingClientRect().height;
    if (tx < 8) tx = 8;
    if (tx + tw > cw - 8) tx = cw - tw - 8;
    if (ty < 8) { ty = y + 20; } // flip below if no room above

    ctx.fillStyle = bgColor;
    ctx.globalAlpha = 0.94;
    ctx.shadowColor = 'rgba(0,0,0,0.4)';
    ctx.shadowBlur = 12;
    ctx.beginPath();
    ctx.roundRect(tx, ty, tw, th, 8);
    ctx.fill();
    ctx.shadowBlur = 0;
    ctx.globalAlpha = 1;
    ctx.strokeStyle = textColor;
    ctx.globalAlpha = 0.15;
    ctx.lineWidth = 1;
    ctx.stroke();
    ctx.globalAlpha = 1;
    ctx.fillStyle = textColor;
    ctx.textAlign = 'left';
    ctx.textBaseline = 'top';
    lines.forEach(function(l, i) {
      ctx.globalAlpha = i === 0 ? 1 : 0.7;
      ctx.fillText(l, tx + pad, ty + pad + i * lineH);
    });
    ctx.restore();
  }

  // ── Screen → world coordinate conversion ─────────────────────────────
  function screenToWorld(sx, sy) {
    var rect = canvas.getBoundingClientRect();
    var W = rect.width, H = rect.height;
    return {
      x: (sx - W / 2 - _panX) / _zoom + W / 2,
      y: (sy - H / 2 - _panY) / _zoom + H / 2
    };
  }

  // Hit-test nodes at screen coordinates. Returns node or null.
  function hitNode(sx, sy) {
    if (!canvas._nodes) return null;
    var w = screenToWorld(sx, sy);
    for (var i = canvas._nodes.length - 1; i >= 0; i--) {
      var n = canvas._nodes[i];
      var dx = w.x - n.x, dy = w.y - n.y;
      if (dx * dx + dy * dy <= (n.r + 6) * (n.r + 6)) return n;
    }
    return null;
  }

  // ── Mouse: hover, drag nodes, drag-to-pan, wheel-to-zoom ─────────────
  canvas.addEventListener('mousemove', function(e) {
    var rect = canvas.getBoundingClientRect();
    var sx = e.clientX - rect.left, sy = e.clientY - rect.top;
    _hover = { x: sx, y: sy };

    if (_dragNode) {
      // Dragging a node — update its manual position in world coords.
      var w = screenToWorld(sx, sy);
      var id = _dragNode.peer.id || (_dragNode.type === 'self' ? 'self' : '');
      if (id) _manualPos[id] = { x: w.x, y: w.y };
    } else if (_dragging && _dragStart) {
      // Panning the canvas.
      _panX = _panStart.px + (e.clientX - _dragStart.x);
      _panY = _panStart.py + (e.clientY - _dragStart.y);
    } else {
      // Hover cursor hint.
      canvas.style.cursor = hitNode(sx, sy) ? 'grab' : '';
    }
  });
  canvas.addEventListener('mouseleave', function() {
    _hover = null;
    _dragging = false;
    _dragNode = null;
    canvas.style.cursor = '';
  });
  canvas.addEventListener('mousedown', function(e) {
    if (e.button !== 0) return;
    var rect = canvas.getBoundingClientRect();
    var sx = e.clientX - rect.left, sy = e.clientY - rect.top;
    var hit = hitNode(sx, sy);
    if (hit) {
      // Start dragging a node.
      _dragNode = hit;
      canvas.style.cursor = 'grabbing';
    } else {
      // Start panning.
      _dragging = true;
      _dragStart = { x: e.clientX, y: e.clientY };
      _panStart = { px: _panX, py: _panY };
      canvas.style.cursor = 'grabbing';
    }
  });
  canvas.addEventListener('mouseup', function() {
    _dragging = false;
    _dragNode = null;
    canvas.style.cursor = '';
  });
  canvas.addEventListener('wheel', function(e) {
    e.preventDefault();
    var delta = e.deltaY > 0 ? 0.9 : 1.1;
    var newZoom = _zoom * delta;
    if (newZoom < 0.3) newZoom = 0.3;
    if (newZoom > 4) newZoom = 4;
    var rect = canvas.getBoundingClientRect();
    var mx = e.clientX - rect.left;
    var my = e.clientY - rect.top;
    _panX = mx - (mx - _panX) * (newZoom / _zoom);
    _panY = my - (my - _panY) * (newZoom / _zoom);
    _zoom = newZoom;
  }, { passive: false });
  // Double-click to reset zoom/pan and manual positions.
  canvas.addEventListener('dblclick', function() {
    _zoom = 1; _panX = 0; _panY = 0; _manualPos = {};
  });
})();
