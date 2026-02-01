-- weather.lua — Fetch weather from OpenWeatherMap. Requires API key in KV.
-- Set the key first: goop.kv.set("api_key", "YOUR_KEY") via another script.
function handle(args)
    if args == "" then
        return "Usage: !weather <city>"
    end

    local key = goop.kv.get("api_key")
    if not key then
        return "Weather API key not configured. Owner must set it."
    end

    local url = "https://api.openweathermap.org/data/2.5/weather"
        .. "?q=" .. args
        .. "&appid=" .. key
        .. "&units=metric"

    local status, body, err = goop.http.get(url)
    if err then
        return "Error fetching weather: " .. err
    end
    if status ~= 200 then
        return "Weather API returned status " .. tostring(status)
    end

    local data = goop.json.decode(body)
    return string.format("%s: %s°C, %s",
        data.name,
        tostring(math.floor(data.main.temp)),
        data.weather[1].description
    )
end
