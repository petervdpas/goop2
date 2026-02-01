-- roll.lua — Roll dice. Usage: !roll [N] (default 6)
function handle(args)
    local sides = tonumber(args)
    if not sides or sides < 1 then
        sides = 6
    end
    math.randomseed(os.time())
    local result = math.random(1, math.floor(sides))
    return goop.peer.label .. " rolled a d" .. sides .. ": " .. result
end
