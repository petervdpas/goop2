-- about.lua — Info about this peer's bot.
function handle(args)
    return "I am " .. goop.self.label .. " (" .. goop.self.id .. ").\n" ..
           "I run Lua chat scripts. Try !help for commands."
end
