--- Test: goop.config() with single-row settings table
--- @rate_limit 0

local cfg = nil

function call(req)
    if not cfg then
        cfg = goop.config("app_settings", {
            api_url = "https://default.example.com",
            timeout = 30,
            debug = "false",
        })
    end

    local action = req.params.action

    if action == "read" then
        return {
            api_url = cfg.api_url,
            timeout = cfg.timeout,
            debug = cfg.debug,
        }

    elseif action == "set" then
        cfg:set(req.params.key, req.params.value)
        return { ok = true }

    elseif action == "save" then
        cfg:save(req.params.data)
        return { ok = true }

    elseif action == "read_all" then
        return {
            api_url = cfg.api_url,
            timeout = cfg.timeout,
            debug = cfg.debug,
        }

    else
        error("unknown action: " .. tostring(action))
    end
end
