package com.antigravity.mobile.data.model

import com.antigravity.mobile.ui.util.loc
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class PairingInfo(
    val host: String,
    val port: Int,
    val code: String,
    val ssl: Boolean = false,
    val lanHost: String? = null,
    val ddnsHost: String? = null,
    val relayHost: String? = null,
    val os: String? = null,
    val platform: String? = null,
    val e2eePubKey: String? = null
) {
    val serverBaseUrl: String
        get() = formatUrl(host, port, ssl)

    val lanBaseUrl: String?
        get() {
            val lan = lanHost?.takeIf { it.isNotBlank() } ?: return null
            val lanPort = if (ssl && port == 443) 58900 else port
            val lanSsl = if (ssl && port == 443) false else ssl
            return formatUrl(lan, lanPort, lanSsl)
        }

    val ddnsBaseUrl: String?
        get() = ddnsHost?.takeIf { it.isNotBlank() }?.let { formatUrl(it, port, ssl) }

    val relayBaseUrl: String?
        get() = relayHost?.takeIf { it.isNotBlank() }?.let { formatUrl(it, port, ssl) }

    fun candidateBaseUrls(): List<String> {
        val list = mutableListOf<String>()
        lanBaseUrl?.let { if (!list.contains(it)) list.add(it) }
        val primary = serverBaseUrl
        if (!list.contains(primary)) list.add(primary)
        relayBaseUrl?.let { if (!list.contains(it)) list.add(it) }
        ddnsBaseUrl?.let { if (!list.contains(it)) list.add(it) }
        return list
    }

    companion object {
        fun formatUrl(host: String, port: Int, ssl: Boolean): String {
            val scheme = if (ssl) "https://" else "http://"
            var formattedHost = host.trim()
            if (!formattedHost.startsWith("[") && formattedHost.count { it == ':' } >= 2) {
                formattedHost = "[$formattedHost]"
            }
            if ((ssl && port == 443) || (!ssl && port == 80)) {
                return "$scheme$formattedHost"
            }
            return "$scheme$formattedHost:$port"
        }

        private fun isPairingUriChar(ch: Char): Boolean {
            return ch in '0'..'9' ||
                ch in 'A'..'Z' ||
                ch in 'a'..'z' ||
                ch == '-' || ch == '.' || ch == '_' || ch == '~' ||
                ch == '%' || ch == '+' || ch == '&' || ch == '=' ||
                ch == ':' || ch == '/' || ch == '?' ||
                ch == '[' || ch == ']'
        }

        /**
         * Tolerantly extracts the first pairing URI (`agy://pair?...`, `multigravity://pair?...`,
         * or `antigravity://pair?...`) from raw clipboard/terminal text, handling prefixes such as
         * `🔗 配对 URI:` and terminal hard line wraps without absorbing trailing prose.
         */
        fun extractPairingUri(rawText: String): String? {
            val normalized = rawText.replace("\r\n", "\n").replace('\r', '\n')
            val lines = normalized.split('\n')
            val prefixes = listOf("agy://pair?", "multigravity://pair?", "antigravity://pair?")

            var startLineIdx = -1
            var startCol = -1
            for (i in lines.indices) {
                val lower = lines[i].lowercase()
                var earliest = -1
                for (prefix in prefixes) {
                    val idx = lower.indexOf(prefix)
                    if (idx >= 0 && (earliest < 0 || idx < earliest)) {
                        earliest = idx
                    }
                }
                if (earliest >= 0) {
                    startLineIdx = i
                    startCol = earliest
                    break
                }
            }
            if (startLineIdx < 0 || startCol < 0) return null

            val firstLine = lines[startLineIdx]
            var endCol = startCol
            while (endCol < firstLine.length && isPairingUriChar(firstLine[endCol])) {
                endCol++
            }
            val sb = StringBuilder(firstLine.substring(startCol, endCol))
            val remainderAfterUri = firstLine.substring(endCol).trim()

            // Stitch wrapped continuation lines only when the URI reached the end of the line.
            if (remainderAfterUri.isEmpty()) {
                var prevLineLength = firstLine.trim().length
                var nextIdx = startLineIdx + 1
                while (nextIdx < lines.size) {
                    val rawNext = lines[nextIdx].trim()
                    if (rawNext.isEmpty()) break
                    if (!rawNext.all { isPairingUriChar(it) }) break

                    val accLower = sb.toString().lowercase()
                    val missingCoreParam = !accLower.contains("host=") || !accLower.contains("code=")
                    val looksLikeQueryContinuation = rawNext.startsWith("&") || rawNext.contains("=")
                    val likelyTerminalWrap = prevLineLength >= 32
                    if (missingCoreParam || looksLikeQueryContinuation || likelyTerminalWrap) {
                        sb.append(rawNext)
                        prevLineLength = rawNext.length
                        nextIdx++
                    } else {
                        break
                    }
                }
            }

            return sb.toString().takeIf { it.isNotEmpty() }
        }

        fun parseFromUri(uriString: String): PairingInfo? {
            val trimmed = extractPairingUri(uriString) ?: uriString.trim()
            val lower = trimmed.lowercase()
            if (!lower.startsWith("agy://pair") &&
                !lower.startsWith("multigravity://pair") &&
                !lower.startsWith("antigravity://pair")
            ) {
                return null
            }

            val queryPart = trimmed.substringAfter('?', "")
            if (queryPart.isEmpty()) return null

            val params = queryPart.split("&").associate {
                val pair = it.split("=", limit = 2)
                val key = pair[0].lowercase()
                val value = if (pair.size > 1) pair[1] else ""
                key to java.net.URLDecoder.decode(value, "UTF-8")
            }

            val rawHost = params["host"] ?: return null
            val port = params["port"]?.toIntOrNull() ?: 443
            val code = params["code"] ?: return null
            val lan = params["lan"]
            val ddns = params["ddns"]
            val relay = params["relay"]
            val os = params["os"]
            val platform = params["platform"]
            val e2eePubKey = params["pk"]?.trim()?.takeIf { it.isNotEmpty() }

            var host = rawHost.trim()
            if (!host.contains(".") && !host.contains(":") && !host.equals("localhost", ignoreCase = true)) {
                host = "$host.jiuge.space"
            }

            val ssl = params["ssl"]?.let { it == "1" || it.equals("true", ignoreCase = true) }
                ?: (port == 443 || (!host.startsWith("192.168.") && !host.startsWith("10.") && !host.startsWith("172.") && !host.startsWith("127.")))

            var cleanRelay = relay?.trim()
            if (cleanRelay != null && !cleanRelay.contains(".") && !cleanRelay.contains(":") && !cleanRelay.equals("localhost", ignoreCase = true)) {
                cleanRelay = "$cleanRelay.jiuge.space"
            }

            return PairingInfo(
                host = host,
                port = port,
                code = code,
                ssl = ssl,
                lanHost = lan,
                ddnsHost = ddns,
                relayHost = cleanRelay,
                os = os,
                platform = platform,
                e2eePubKey = e2eePubKey
            )
        }
    }

    val gatewayDisplayName: String
        get() = when ((platform ?: os)?.lowercase()) {
            "windows", "win" -> "Windows 网关".loc
            "darwin", "macos", "mac" -> "Mac 网关".loc
            "linux" -> "Linux 网关".loc
            else -> "网关".loc
        }
}

@Serializable
data class PairRequest(
    @SerialName("pairing_code") val pairingCode: String,
    @SerialName("device_name") val deviceName: String,
    @SerialName("platform") val platform: String = "android",
    @SerialName("e2ee_pub") val e2eePub: String? = null
)

@Serializable
data class EndpointInfo(
    val type: String,
    val url: String
)

@Serializable
data class PairResponse(
    @SerialName("device_id") val deviceId: String,
    @SerialName("device_token") val deviceToken: String,
    val endpoints: List<EndpointInfo>? = null,
    val os: String? = null,
    val platform: String? = null,
    @SerialName("gateway_id") val gatewayId: String? = null,
    @SerialName("gateway_name") val gatewayName: String? = null,
    @SerialName("e2ee_gateway_pub") val e2eeGatewayPub: String? = null,
    @SerialName("e2ee_salt") val e2eeSalt: String? = null,
    @SerialName("relay_cloud_url") val relayCloudUrl: String? = null
)

@Serializable
data class WsTicketResponse(
    val ticket: String = "",
    @SerialName("expires_in") val expiresIn: Int = 30
)
