-- counter.lua — Per-peer visit counter using goop.kv.
function handle(args)
    local key = "count:" .. goop.peer.id
    local count = goop.kv.get(key) or 0
    count = count + 1
    goop.kv.set(key, count)

    local total_key = "total"
    local total = goop.kv.get(total_key) or 0
    total = total + 1
    goop.kv.set(total_key, total)

    return goop.peer.label .. ", you've used !counter " .. count ..
           " time(s). Total across all peers: " .. total
end
