# FAQ & Troubleshooting

## General questions

### Is Goop2 free?

Yes. Goop2 is free, open-source software. An optional credit system allows rendezvous operators to offer premium templates through a built-in template store, but the application itself and all core functionality are free to use.

### Do I need a server?

No. A peer runs on your own machine. A rendezvous server is optional and only needed if you want peers on different networks to discover each other.

### What happens when I shut down?

Your content vanishes from the network. Other peers will mark you as offline after the TTL expires (default 20 seconds). When you start up again, your identity and data are still on disk.

### Can I run multiple peers on one machine?

Yes. Give each peer a different directory and a different `viewer.http_addr` port. If both peers use libp2p, set different `p2p.listen_port` values (or leave both at `0` for auto-selection).

### What are the external services?

Goop2 can optionally connect to six standalone microservices (registration, credits, email, templates, bridge, encryption) that add functionality to the rendezvous server. They are separate binaries, not part of the core. Set `use_services` to `true` and provide service URLs to enable them. See [Configuration](configuration) for details.

## Networking

### Peers on the same LAN can't find each other

- **Check your firewall.** mDNS requires UDP port 5353 to be open.
  ```bash
  sudo ufw allow 5353/udp
  ```
- **Check the mDNS tag.** All peers must share the same `p2p.mdns_tag` value.
- **Check network isolation.** VMs, Docker containers, and different subnets may prevent mDNS from working. Use bridged networking or a rendezvous server instead.

### Peers can't reach the rendezvous server

- **Verify the server is running.**
  ```bash
  curl http://<server-address>/healthz
  ```
- **Check the URL in your config.** The `rendezvous_wan` value must be a full URL including the protocol (`http://` or `https://`).
- **Check the firewall.** The rendezvous port (default 8787) must be reachable, or use a reverse proxy on ports 80/443.
- **Check TLS.** If using HTTPS, make sure your certificate is valid.
- **Check the bind address.** If hosting a rendezvous server, set `rendezvous_bind` to `0.0.0.0` to accept connections from other machines.

### Peers can't connect directly (NAT issues)

If peers discover each other via rendezvous but can't exchange data:

- **Enable the circuit relay.** Set `relay_port` on your rendezvous server (e.g. `4001`). Peers will automatically use it for NAT traversal.
- **Forward a port.** If you can, forward a TCP port on your router and set `p2p.listen_port` to match.
- **Check your router.** Some routers block hole punching. The circuit relay is the fallback for these cases.
- **Try bridge mode.** If P2P is not possible at all, enable `bridge_mode` to connect through a bridge service over WebSocket.

### What ports do I need to open?

| Port | Protocol | Purpose |
|------|----------|---------|
| 5353 | UDP | mDNS (LAN discovery) |
| 4001 (or your `listen_port`) | TCP | libp2p peer connections |
| 4001 (or your `relay_port`) | TCP | Circuit relay (on the rendezvous server) |
| 8787 (or your `rendezvous_port`) | TCP | Rendezvous server |
| 8080 (or your `http_addr` port) | TCP | Local viewer (usually localhost only) |

### Real-time updates aren't working in the rendezvous dashboard

Goop2 uses WebSocket as the primary transport for rendezvous signaling, with SSE as a fallback. If you're behind a reverse proxy, make sure WebSocket upgrade and response buffering are configured:

**Nginx:**
```nginx
proxy_http_version 1.1;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
proxy_buffering off;
proxy_read_timeout 86400;
```

**Caddy** handles both WebSocket and SSE correctly by default.

## Video calls

### Video calls aren't working

- Native video calls are currently **Linux only**. They are enabled automatically when running the desktop app on Linux.
- Check that `video_disabled` is `false` in your config.
- Check that a camera device is available. Goop2 skips audio capture if no audio device is found.
- Both peers must be directly connected (via LAN, rendezvous, or relay). Call signaling happens over the MQ bus.

### Video freezes or reconnects repeatedly

- This is typically caused by subscriber channel backpressure. Make sure your network can handle the video bitrate.
- On page navigation, calls are restored automatically. The viewer requests a fresh keyframe (PLI) to avoid stale-frame freezes.

### Can I set preferred camera/mic?

Yes. Set `preferred_cam` and `preferred_mic` in the viewer config to the device labels you want. These are also configurable from the settings page.

## Cluster compute

### How do I set up a cluster?

1. Configure an executor binary on worker peers (`cluster_binary_path` + `cluster_binary_mode`).
2. One peer creates a cluster from the Groups page.
3. Other peers join the cluster.
4. The host submits jobs; workers execute them.

### What executor binary modes are there?

- **oneshot**: Binary is started per job and exits after producing a result.
- **daemon**: Binary starts once and handles multiple jobs via stdin/stdout JSON.

See the [Executor Protocol](executor) page for the full specification and code examples.

## File sharing

### How do I share files with other peers?

1. Create a file group from the **Groups** page.
2. Upload files via the file sharing UI (max 50 MB per file).
3. Other group members can browse and download your files.

### Where are shared files stored?

Files are stored locally on each peer's disk. There is no central file server. When browsing, the viewer queries all online group members and merges their file lists. Downloads stream directly from the file owner.

## Site content

### My site shows a blank page

Check that your `site/` directory contains an `index.html` file. The site root is set by `paths.site_root` in your config.

### Remote data operations aren't working

- Verify your template includes `<script src="/sdk/goop-data.js"></script>`.
- Check the browser console for errors.
- Make sure the remote peer is still online.
- Review the peer's logs for Lua errors if data functions are involved.

## Lua scripting

### Scripts aren't running

- Check that `lua.enabled` is `true` in your config.
- Verify scripts are in the correct directory (`site/lua/` for chat commands, `site/lua/functions/` for data functions).
- Check the logs for compilation errors.
- Chat commands must export a `handle(args)` function. Data functions must export a `call(request)` function.

### Script changes aren't taking effect

Scripts hot-reload on file changes. If a change doesn't appear, check the logs for syntax errors. The previous working version stays active when there's a compilation error.

## Performance

### High memory usage

- **Lua**: Reduce `lua.max_memory_mb` in your config.
- **Database**: Large databases consume memory. Archive old data if needed.
- **Rendezvous**: Many connected peers increase memory usage. Use `peer_db_path` for persistence so the server can restart without losing state.

### Peer starts slowly

- If `listen_port` is `0`, libp2p tries multiple ports. Setting a fixed port avoids this.
- A large `site/` directory takes longer to hash on startup.
- A large database takes longer to open.

## Database

### Where is my data stored?

In `data.db` (SQLite) inside your peer directory. This file is created automatically on first run.

### Can I back up my data?

Yes. Copy the `data.db` file while the peer is stopped, or use SQLite's `.backup` command. Also back up `data/identity.key` to preserve your peer identity.

### Can I reset my peer identity?

Delete `data/identity.key` and restart. A new identity will be generated. Other peers will see you as a different peer.
