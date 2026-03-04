-- greet.lua — Responds differently based on who's calling.
function handle(args)
    local label = goop.peer.label
    if label ~= "" then
        return "Hello, " .. label .. "!"
    else
        return "Hello, peer " .. string.sub(goop.peer.id, 1, 8) .. "...!"
    end
end
