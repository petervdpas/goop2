-- whoami.lua — Tells the caller who they are (as seen by this peer).
function handle(args)
    return "You are " .. goop.peer.label .. " (" .. goop.peer.id .. ")"
end
