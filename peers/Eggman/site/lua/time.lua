-- time.lua — Returns the current time.
function handle(args)
    local now = os.date("!%Y-%m-%d %H:%M:%S UTC")
    return "Current time: " .. now
end
