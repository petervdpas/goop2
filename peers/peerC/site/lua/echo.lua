-- echo.lua — Echoes back whatever the caller says.
function handle(args)
    if args == "" then
        return "Usage: !echo <message>"
    end
    return args
end
