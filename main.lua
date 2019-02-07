local function starts_with(str, start)
    return str:sub(1, #start) == start
 end

function onRequest(url, headers, body)
    headers["Accept-Encoding"] = {"None"}
    return url, headers, body
end

function onResponse(status, headers, body)
    local content_type = headers["Content-Type"] ~= nil and headers["Content-Type"][1] or ""

    if (starts_with(content_type, "text/html") 
        or starts_with(content_type, "application/json")) then
        body = body:gsub("Cloud", "Butt")
        body = body:gsub("cloud", "butt")
    end

    return status, headers, body
end