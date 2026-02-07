-- help.lua — Lists all available commands.
function handle(args)
    local cmds = goop.commands()
    if #cmds == 0 then
        return "No commands loaded."
    end
    local lines = {"Available commands:"}
    for _, name in ipairs(cmds) do
        lines[#lines + 1] = "  !" .. name
    end
    return table.concat(lines, "\n")
end
