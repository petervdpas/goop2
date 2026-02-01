-- pick.lua — Pick a random item. Usage: !pick option1, option2, option3
function handle(args)
    if args == "" then
        return "Usage: !pick option1, option2, option3"
    end
    local items = {}
    for item in string.gmatch(args, "[^,]+") do
        local trimmed = item:match("^%s*(.-)%s*$")
        if trimmed ~= "" then
            items[#items + 1] = trimmed
        end
    end
    if #items == 0 then
        return "No items to pick from."
    end
    math.randomseed(os.time())
    local choice = items[math.random(1, #items)]
    return goop.peer.label .. " picked: " .. choice
end
