--- Test: goop.config() with key-value table
--- @rate_limit 0

local cfg = nil

function call(req)
    if not cfg then
        cfg = goop.config("settings", {
            theme = "light",
            lang = "en",
            accent = "#333",
        })
    end

    local action = req.params.action

    if action == "read" then
        return {
            theme = cfg.theme,
            lang = cfg.lang,
            accent = cfg.accent,
        }

    elseif action == "set" then
        cfg:set(req.params.key, req.params.value)
        return { ok = true }

    elseif action == "read_after_set" then
        cfg:set("theme", "ocean")
        return { theme = cfg.theme }

    else
        error("unknown action: " .. tostring(action))
    end
end
