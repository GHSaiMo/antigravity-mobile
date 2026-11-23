package com.antigravity.mobile.data.service

import android.util.Base64
import android.util.Log
import okhttp3.Headers
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.Buffer
import okio.ByteString
import okio.ByteString.Companion.toByteString
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.math.BigInteger
import java.security.AlgorithmParameters
import java.security.KeyFactory
import java.security.KeyPair
import java.security.KeyPairGenerator
import java.security.PrivateKey
import java.security.SecureRandom
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPublicKeySpec
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import java.util.zip.Inflater
import javax.crypto.Cipher
import javax.crypto.KeyAgreement
import javax.crypto.Mac
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * Zero-knowledge end-to-end encryption (`mgy-e2ee-v1`) matching Go `internal/e2ee` & iOS `E2EECrypto`:
 * - ECDH P-256 (`secp256r1`, 65-byte uncompressed SEC1 `0x04 || X(32) || Y(32)`)
 * - Single-block RFC 5869 HKDF-SHA256 (`HMAC(HMAC(salt, ikm), info || 0x01)`)
 * - AES-256-GCM (`nonce(12B) || ciphertext || tag(16B)`)
 *
 * Works natively on Android API 26+ JCA with zero external cryptography dependencies.
 */
object E2eeCrypto {
    private const val TAG = "E2eeCrypto"
    const val HEADER_DEVICE_ID = "X-Mgy-E2EE-Device"
    const val HEADER_ACCEPT_ENC = "X-Mgy-E2EE-Enc"
    const val SUPPORTED_ENC = "bin2,flate"
    const val FRAME_BIN_RAW: Byte = 0x01
    const val FRAME_BIN_FLATE: Byte = 0x02
    private val PAIR_SALT = "mgy-e2ee-pair-v1".toByteArray(Charsets.UTF_8)

    /** Decompresses RFC 1951 raw deflate payload (matching Go `compress/flate`). */
    fun inflateRaw(data: ByteArray, offset: Int = 0, length: Int = data.size - offset, maxBytes: Int = 64 * 1024 * 1024): ByteArray {
        if (length <= 0) return ByteArray(0)
        val inflater = Inflater(true)
        try {
            inflater.setInput(data, offset, length)
            val out = ByteArrayOutputStream(minOf(length * 3, 64 * 1024))
            val buf = ByteArray(8192)
            while (!inflater.finished()) {
                val n = inflater.inflate(buf)
                if (n > 0) {
                    if (out.size() + n > maxBytes) {
                        throw IOException("E2EE inflated payload exceeded limit")
                    }
                    out.write(buf, 0, n)
                } else if (inflater.needsInput() || inflater.needsDictionary()) {
                    break
                }
            }
            return out.toByteArray()
        } finally {
            inflater.end()
        }
    }
    private val secureRandom = SecureRandom()
    private val e2eeRequiredHosts = ConcurrentHashMap.newKeySet<String>()
    private val upgradeInFlight = AtomicBoolean(false)
    private val lastUpgradeAttemptMs = AtomicLong(0L)

    private val p256Params: ECParameterSpec by lazy {
        val params = AlgorithmParameters.getInstance("EC")
        params.init(ECGenParameterSpec("secp256r1"))
        params.getParameterSpec(ECParameterSpec::class.java)
    }

    fun markHostRequiresE2ee(host: String) {
        val clean = ConnectionManager.extractHost(host)
        if (clean.isNotEmpty()) {
            e2eeRequiredHosts.add(clean)
        }
    }

    fun isRelayHost(host: String): Boolean {
        val clean = ConnectionManager.extractHost(host)
        if (clean.isEmpty() || ConnectionManager.isLanHost(clean) || ConnectionManager.isTailscaleHost(clean)) {
            return false
        }
        if (clean.startsWith("r-")) return true
        return e2eeRequiredHosts.contains(clean)
    }

    fun isReady(prefs: PreferencesManager): Boolean {
        val devId = prefs.deviceId?.trim()
        val key = decodeDeviceKey(prefs)
        return !devId.isNullOrEmpty() && key != null
    }

    fun decodeDeviceKey(prefs: PreferencesManager): ByteArray? {
        val rawB64 = prefs.e2eeKeyB64?.trim()?.takeIf { it.isNotEmpty() } ?: return null
        val bytes = decodeBase64Flexible(rawB64) ?: return null
        return if (bytes.size == 32) bytes else null
    }

    fun shouldEncrypt(scheme: String, host: String, path: String, prefs: PreferencesManager): Boolean {
        if (!scheme.equals("https", ignoreCase = true) && !scheme.equals("wss", ignoreCase = true)) {
            return false
        }
        if (path == "/healthz" || path.startsWith("/api/v1/e2ee/")) {
            return false
        }
        if (!isReady(prefs)) return false
        return isRelayHost(host)
    }

    fun base64UrlEncode(data: ByteArray): String =
        Base64.encodeToString(data, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)

    fun decodeBase64Flexible(raw: String): ByteArray? {
        val trimmed = raw.trim()
        if (trimmed.isEmpty()) return null
        val std = trimmed.replace('-', '+').replace('_', '/')
        val padded = when (std.length % 4) {
            2 -> "$std=="
            3 -> "$std="
            else -> std
        }
        return try {
            Base64.decode(padded, Base64.DEFAULT)
        } catch (_: Exception) {
            null
        }
    }

    private fun toUnsigned32Bytes(bi: BigInteger): ByteArray {
        val raw = bi.toByteArray()
        val out = ByteArray(32)
        when {
            raw.size == 32 -> System.arraycopy(raw, 0, out, 0, 32)
            raw.size > 32 -> System.arraycopy(raw, raw.size - 32, out, 0, 32)
            else -> System.arraycopy(raw, 0, out, 32 - raw.size, raw.size)
        }
        return out
    }

    fun generateEphemeralKeyPair(): Pair<KeyPair, String> {
        val kpg = KeyPairGenerator.getInstance("EC")
        kpg.initialize(ECGenParameterSpec("secp256r1"), secureRandom)
        val kp = kpg.generateKeyPair()
        val pub = kp.public as ECPublicKey
        val w = pub.w
        val sec1 = ByteArray(65)
        sec1[0] = 0x04
        System.arraycopy(toUnsigned32Bytes(w.affineX), 0, sec1, 1, 32)
        System.arraycopy(toUnsigned32Bytes(w.affineY), 0, sec1, 33, 32)
        return kp to base64UrlEncode(sec1)
    }

    fun sharedSecret(privateKey: PrivateKey, peerPubB64: String): ByteArray {
        val raw = decodeBase64Flexible(peerPubB64)
            ?: throw IllegalArgumentException("invalid peer pubkey base64")
        if (raw.size != 65 || raw[0] != 0x04.toByte()) {
            throw IllegalArgumentException("invalid uncompressed P-256 pubkey length=${raw.size}")
        }
        val x = BigInteger(1, raw.copyOfRange(1, 33))
        val y = BigInteger(1, raw.copyOfRange(33, 65))
        val pubSpec = ECPublicKeySpec(ECPoint(x, y), p256Params)
        val peerPub = KeyFactory.getInstance("EC").generatePublic(pubSpec)
        val ka = KeyAgreement.getInstance("ECDH")
        ka.init(privateKey)
        ka.doPhase(peerPub, true)
        return ka.generateSecret()
    }

    fun hkdfSha256(ikm: ByteArray, salt: ByteArray, info: ByteArray): ByteArray {
        val effectiveSalt = if (salt.isEmpty()) ByteArray(32) else salt
        val extract = Mac.getInstance("HmacSHA256")
        extract.init(SecretKeySpec(effectiveSalt, "HmacSHA256"))
        val prk = extract.doFinal(ikm)

        val expand = Mac.getInstance("HmacSHA256")
        expand.init(SecretKeySpec(prk, "HmacSHA256"))
        expand.update(info)
        expand.update(0x01.toByte())
        return expand.doFinal()
    }

    fun derivePairKey(sharedSecret: ByteArray, clientPubB64: String, gatewayPubB64: String): ByteArray {
        val info = "pair:${clientPubB64.trim()}:${gatewayPubB64.trim()}".toByteArray(Charsets.UTF_8)
        return hkdfSha256(sharedSecret, PAIR_SALT, info)
    }

    fun deriveDeviceKey(sharedSecret: ByteArray, salt: ByteArray, deviceId: String): ByteArray {
        val info = "mgy-e2ee-device-v1:${deviceId.trim()}".toByteArray(Charsets.UTF_8)
        return hkdfSha256(sharedSecret, salt, info)
    }

    fun frameAad(deviceId: String, direction: String): ByteArray =
        "mgy-e2ee-v1:${deviceId.trim()}:$direction".toByteArray(Charsets.UTF_8)

    fun pairFrameAad(clientPubB64: String, direction: String): ByteArray =
        "mgy-e2ee-pair-v1:${clientPubB64.trim()}:$direction".toByteArray(Charsets.UTF_8)

    fun encrypt(key: ByteArray, plaintext: ByteArray, aad: ByteArray): ByteArray {
        require(key.size == 32) { "invalid AES-256 key size" }
        val nonce = ByteArray(12)
        secureRandom.nextBytes(nonce)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, nonce))
        cipher.updateAAD(aad)
        val ct = cipher.doFinal(plaintext)
        val out = ByteArray(nonce.size + ct.size)
        System.arraycopy(nonce, 0, out, 0, nonce.size)
        System.arraycopy(ct, 0, out, nonce.size, ct.size)
        return out
    }

    fun decrypt(key: ByteArray, frame: ByteArray, aad: ByteArray): ByteArray {
        require(key.size == 32) { "invalid AES-256 key size" }
        if (frame.size < 28) throw IllegalArgumentException("e2ee frame too short")
        val nonce = frame.copyOfRange(0, 12)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, nonce))
        cipher.updateAAD(aad)
        return cipher.doFinal(frame, 12, frame.size - 12)
    }

    fun deriveAndSaveDeviceKey(
        privateKey: PrivateKey,
        gatewayPubB64: String,
        saltB64: String,
        deviceId: String,
        prefs: PreferencesManager
    ): Boolean {
        return try {
            val salt = decodeBase64Flexible(saltB64) ?: return false
            if (salt.isEmpty()) return false
            val shared = sharedSecret(privateKey, gatewayPubB64)
            val devKey = deriveDeviceKey(shared, salt, deviceId)
            prefs.e2eeKeyB64 = base64UrlEncode(devKey)
            true
        } catch (e: Exception) {
            Log.w(TAG, "deriveAndSaveDeviceKey failed: ${e.message}")
            false
        }
    }

    /**
     * Encrypts a pairing JSON payload into the `POST /api/v1/e2ee/pair` outer envelope.
     */
    fun encryptPairEnvelope(
        pairBodyBytes: ByteArray,
        gatewayPubB64: String
    ): Triple<KeyPair, String, String> {
        val (kp, clientPubB64) = generateEphemeralKeyPair()
        val shared = sharedSecret(kp.private, gatewayPubB64)
        val pairKey = derivePairKey(shared, clientPubB64, gatewayPubB64)
        val cipherBytes = encrypt(pairKey, pairBodyBytes, pairFrameAad(clientPubB64, "c2s"))
        val env = JSONObject().apply {
            put("client_pub", clientPubB64)
            put("ciphertext", Base64.encodeToString(cipherBytes, Base64.NO_WRAP))
        }
        return Triple(kp, clientPubB64, env.toString())
    }

    /**
     * Decrypts the response from `POST /api/v1/e2ee/pair` into `(statusCode, rawBodyJson)`.
     */
    fun decryptPairEnvelope(
        outerJson: String,
        privateKey: PrivateKey,
        clientPubB64: String,
        gatewayPubB64: String
    ): Pair<Int, String> {
        val outerObj = JSONObject(outerJson)
        val cipherB64 = outerObj.getString("ciphertext")
        val cipherBytes = decodeBase64Flexible(cipherB64)
            ?: throw IOException("invalid pair ciphertext base64")
        val shared = sharedSecret(privateKey, gatewayPubB64)
        val pairKey = derivePairKey(shared, clientPubB64, gatewayPubB64)
        val plain = decrypt(pairKey, cipherBytes, pairFrameAad(clientPubB64, "s2c"))
        val wrapped = JSONObject(String(plain, Charsets.UTF_8))
        val status = wrapped.optInt("status", 200)
        val bodyStr = wrapped.opt("body")?.toString() ?: "{}"
        return status to bodyStr
    }

    /**
     * Silently upgrades an already-paired pre-1.2.5 Android device to E2EE over LAN or legacy CF Tunnel
     * without requiring a QR re-scan.
     */
    fun silentUpgradeIfNeeded(
        reachableBaseUrl: String,
        prefs: PreferencesManager,
        httpClient: OkHttpClient
    ) {
        if (isReady(prefs)) return
        val token = prefs.deviceToken?.trim()?.takeIf { it.isNotEmpty() } ?: return
        val now = System.currentTimeMillis()
        if (now - lastUpgradeAttemptMs.get() < 60_000L) return
        if (!upgradeInFlight.compareAndSet(false, true)) return
        lastUpgradeAttemptMs.set(now)

        try {
            val cleanBase = reachableBaseUrl.trim().trimEnd('/')
            val (kp, clientPubB64) = generateEphemeralKeyPair()
            val bodyJson = JSONObject().put("client_pub", clientPubB64).toString()
            val req = Request.Builder()
                .url("$cleanBase/api/v1/auth/e2ee/upgrade")
                .header("Authorization", "Bearer $token")
                .post(bodyJson.toRequestBody("application/json; charset=utf-8".toMediaType()))
                .build()
            httpClient.newCall(req).execute().use { resp ->
                if (!resp.isSuccessful) return
                val respStr = resp.body?.string().orEmpty()
                if (respStr.isBlank()) return
                val obj = JSONObject(respStr)
                val deviceId = obj.optString("device_id").trim()
                val gwPub = obj.optString("gateway_pub").trim()
                val saltB64 = obj.optString("e2ee_salt").trim()
                val cloudUrl = obj.optString("cloud_url").trim().trimEnd('/')
                if (deviceId.isEmpty() || gwPub.isEmpty() || saltB64.isEmpty()) return

                prefs.deviceId = deviceId
                if (deriveAndSaveDeviceKey(kp.private, gwPub, saltB64, deviceId, prefs)) {
                    if (cloudUrl.isNotEmpty()) {
                        val h = ConnectionManager.extractHost(cloudUrl)
                        if (h.startsWith("r-")) {
                            markHostRequiresE2ee(h)
                        }
                        if (ConnectionManager.isValidAdvertisedCloudUrl(cloudUrl, prefs, trustedByE2ee = true)) {
                            prefs.primaryCloudUrl = cloudUrl
                        }
                    }
                    Log.i(TAG, "Silent E2EE upgrade succeeded for device $deviceId")
                }
            }
        } catch (e: Exception) {
            Log.d(TAG, "Silent E2EE upgrade skipped/deferred: ${e.message}")
        } finally {
            upgradeInFlight.set(false)
        }
    }
}

/**
 * Transparent OkHttp interceptor that encrypts outgoing HTTP requests targeting E2EE relay hosts
 * into `POST /api/v1/e2ee/rpc` and decrypts responses back into normal OkHttp `Response` objects.
 * Also auto-recovers on `HTTP 426 e2ee_required` for custom-domain relay nodes.
 */
class E2eeTransportInterceptor(
    private val prefs: PreferencesManager,
    private val connectionManager: ConnectionManager? = null
) : Interceptor {
    private val octetMediaType = "application/octet-stream".toMediaType()

    override fun intercept(chain: Interceptor.Chain): Response {
        val original = chain.request()
        val url = original.url
        if (E2eeCrypto.shouldEncrypt(url.scheme, url.host, url.encodedPath, prefs)) {
            return executeEncrypted(chain, original)
        }
        val resp = chain.proceed(original)
        if (resp.code == 426 &&
            E2eeCrypto.isReady(prefs) &&
            url.isHttps &&
            !ConnectionManager.isLanHost(url.host) &&
            !ConnectionManager.isTailscaleHost(url.host) &&
            !url.encodedPath.startsWith("/api/v1/e2ee/")
        ) {
            resp.close()
            E2eeCrypto.markHostRequiresE2ee(url.host)
            return executeEncrypted(chain, original)
        }
        return resp
    }

    private fun executeEncrypted(chain: Interceptor.Chain, original: Request): Response {
        val deviceId = prefs.deviceId?.trim()?.takeIf { it.isNotEmpty() }
            ?: throw IOException("E2EE device_id missing")
        val key = E2eeCrypto.decodeDeviceKey(prefs)
            ?: throw IOException("E2EE device key missing")

        val url = original.url
        val pathWithQuery = buildString {
            append(url.encodedPath.ifEmpty { "/" })
            val q = url.encodedQuery
            if (!q.isNullOrEmpty()) {
                append('?').append(q)
            }
        }

        val headersJson = JSONObject()
        var hasAuth = false
        for (i in 0 until original.headers.size) {
            val name = original.headers.name(i)
            if (name.equals("Accept-Encoding", ignoreCase = true)) continue
            if (name.equals("Authorization", ignoreCase = true)) hasAuth = true
            headersJson.put(name, original.headers.value(i))
        }
        if (!hasAuth) {
            prefs.deviceToken?.trim()?.takeIf { it.isNotEmpty() }?.let { tok ->
                headersJson.put("Authorization", "Bearer $tok")
            }
        }

        var bodyB64: String? = null
        original.body?.let { reqBody ->
            val buf = Buffer()
            reqBody.writeTo(buf)
            val rawBytes = buf.readByteArray()
            if (rawBytes.isNotEmpty()) {
                bodyB64 = Base64.encodeToString(rawBytes, Base64.NO_WRAP)
            }
            reqBody.contentType()?.toString()?.let { ct ->
                if (!headersJson.has("Content-Type")) {
                    headersJson.put("Content-Type", ct)
                }
            }
        }

        val innerJson = JSONObject().apply {
            put("m", original.method.uppercase())
            put("p", pathWithQuery)
            if (headersJson.length() > 0) put("h", headersJson)
            if (bodyB64 != null) put("b", bodyB64)
        }

        val cipherFrame = E2eeCrypto.encrypt(
            key = key,
            plaintext = innerJson.toString().toByteArray(Charsets.UTF_8),
            aad = E2eeCrypto.frameAad(deviceId, "c2s")
        )

        val outerUrl = url.newBuilder()
            .encodedPath("/api/v1/e2ee/rpc")
            .query(null)
            .build()
        val outerReq = Request.Builder()
            .url(outerUrl)
            .header(E2eeCrypto.HEADER_DEVICE_ID, deviceId)
            .header(E2eeCrypto.HEADER_ACCEPT_ENC, E2eeCrypto.SUPPORTED_ENC)
            .post(cipherFrame.toRequestBody(octetMediaType))
            .build()

        val outerResp = chain.proceed(outerReq)
        if (outerResp.code != 200) {
            return outerResp
        }
        val encBytes = outerResp.body?.bytes() ?: ByteArray(0)
        val protocol = outerResp.protocol
        outerResp.close()

        val plainBytes = try {
            E2eeCrypto.decrypt(key, encBytes, E2eeCrypto.frameAad(deviceId, "s2c"))
        } catch (e: Exception) {
            throw IOException("E2EE response decrypt failed: ${e.message}", e)
        }
        if (plainBytes.isEmpty()) {
            throw IOException("E2EE response empty")
        }

        val statusCode: Int
        val headersObj: JSONObject?
        val respBodyBytes: ByteArray

        val firstByte = plainBytes[0]
        if (firstByte == E2eeCrypto.FRAME_BIN_RAW || firstByte == E2eeCrypto.FRAME_BIN_FLATE) {
            val unpacked = if (firstByte == E2eeCrypto.FRAME_BIN_FLATE) {
                E2eeCrypto.inflateRaw(plainBytes, 1, plainBytes.size - 1)
            } else {
                plainBytes.copyOfRange(1, plainBytes.size)
            }
            if (unpacked.size < 4) {
                throw IOException("E2EE bin2 response too short")
            }
            val metaLen = ((unpacked[0].toInt() and 0xFF) shl 24) or
                ((unpacked[1].toInt() and 0xFF) shl 16) or
                ((unpacked[2].toInt() and 0xFF) shl 8) or
                (unpacked[3].toInt() and 0xFF)
            if (metaLen < 2 || 4 + metaLen > unpacked.size) {
                throw IOException("E2EE bin2 invalid meta length=$metaLen")
            }
            val meta = JSONObject(String(unpacked, 4, metaLen, Charsets.UTF_8))
            statusCode = meta.optInt("s", 200)
            headersObj = meta.optJSONObject("h")
            respBodyBytes = unpacked.copyOfRange(4 + metaLen, unpacked.size)
        } else {
            val decoded = JSONObject(String(plainBytes, Charsets.UTF_8))
            statusCode = decoded.optInt("s", 200)
            headersObj = decoded.optJSONObject("h")
            respBodyBytes = decoded.optString("b", "").takeIf { it.isNotEmpty() }?.let { b64 ->
                Base64.decode(b64, Base64.DEFAULT)
            } ?: ByteArray(0)
        }

        val respHeadersBuilder = Headers.Builder()
        var contentType = "application/json; charset=utf-8"
        headersObj?.let { hObj ->
            val keys = hObj.keys()
            while (keys.hasNext()) {
                val k = keys.next()
                val v = hObj.optString(k)
                respHeadersBuilder.add(k, v)
                if (k.equals("Content-Type", ignoreCase = true) && v.isNotBlank()) {
                    contentType = v
                }
                if (k.equals("X-Antigravity-Cloud-URL", ignoreCase = true) && v.isNotBlank()) {
                    ConnectionManager.applyAdvertisedCloudUrl(
                        rawHeader = v,
                        prefs = prefs,
                        trustedByE2ee = true,
                        connectionManager = connectionManager
                    )
                }
            }
        }

        return Response.Builder()
            .request(original)
            .protocol(if (protocol == Protocol.HTTP_1_0) Protocol.HTTP_1_1 else protocol)
            .code(statusCode)
            .message(if (statusCode in 200..299) "OK" else "HTTP $statusCode")
            .headers(respHeadersBuilder.build())
            .body(respBodyBytes.toResponseBody(contentType.toMediaType()))
            .build()
    }
}

/**
 * Transparent WebSocket E2EE tunnel helper (`GET /api/v1/e2ee/ws?d=<device_id>`).
 * When the target host is an E2EE relay host, rewrites the WebSocket connection to `/api/v1/e2ee/ws`,
 * sends encrypted Frame 0 (`WSOpenFrame`) on connect, encrypts outgoing messages, and decrypts
 * incoming binary frames before forwarding to the caller's `WebSocketListener`.
 */
object E2eeWebSocketHelper {
    fun openWebSocket(
        client: OkHttpClient,
        request: Request,
        prefs: PreferencesManager,
        listener: WebSocketListener
    ): WebSocket {
        val url = request.url
        val effectiveScheme = when {
            url.scheme.equals("https", ignoreCase = true) -> "wss"
            url.scheme.equals("http", ignoreCase = true) -> "ws"
            else -> url.scheme
        }
        if (!E2eeCrypto.shouldEncrypt(effectiveScheme, url.host, url.encodedPath, prefs)) {
            return client.newWebSocket(request, listener)
        }

        val deviceId = prefs.deviceId?.trim() ?: return client.newWebSocket(request, listener)
        val key = E2eeCrypto.decodeDeviceKey(prefs) ?: return client.newWebSocket(request, listener)

        val innerPath = buildString {
            append(url.encodedPath.ifEmpty { "/" })
            val q = url.encodedQuery
            if (!q.isNullOrEmpty()) append('?').append(q)
        }
        val innerHeaders = JSONObject()
        var hasAuth = false
        for (i in 0 until request.headers.size) {
            val k = request.headers.name(i)
            if (k.equals("Authorization", ignoreCase = true)) hasAuth = true
            innerHeaders.put(k, request.headers.value(i))
        }
        if (!hasAuth) {
            prefs.deviceToken?.trim()?.takeIf { it.isNotEmpty() }?.let { tok ->
                innerHeaders.put("Authorization", "Bearer $tok")
            }
        }

        val outerUrl = url.newBuilder()
            .scheme("https")
            .encodedPath("/api/v1/e2ee/ws")
            .query(null)
            .addQueryParameter("d", deviceId)
            .build()

        val outerReq = Request.Builder().url(outerUrl).build()

        lateinit var wrapper: E2eeWebSocketWrapper
        val rawWs = client.newWebSocket(outerReq, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                try {
                    val openObj = JSONObject().apply {
                        put("path", innerPath)
                        if (innerHeaders.length() > 0) put("headers", innerHeaders)
                        put("enc", "flate")
                    }
                    val frame0 = E2eeCrypto.encrypt(
                        key = key,
                        plaintext = openObj.toString().toByteArray(Charsets.UTF_8),
                        aad = E2eeCrypto.frameAad(deviceId, "ws-open")
                    )
                    webSocket.send(frame0.toByteString())
                    listener.onOpen(wrapper, response)
                } catch (e: Exception) {
                    webSocket.close(1008, "e2ee_open_failed")
                    listener.onFailure(wrapper, e, response)
                }
            }

            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                try {
                    val plain = E2eeCrypto.decrypt(
                        key = key,
                        frame = bytes.toByteArray(),
                        aad = E2eeCrypto.frameAad(deviceId, "ws-s2c")
                    )
                    if (plain.isEmpty()) return
                    val headerByte = plain[0].toInt() and 0xFF
                    val isCompressed = (headerByte and 0x80) != 0
                    val msgType = headerByte and 0x7F
                    val payload = if (isCompressed) {
                        E2eeCrypto.inflateRaw(plain, 1, plain.size - 1)
                    } else {
                        plain.copyOfRange(1, plain.size)
                    }
                    if (msgType == 1) {
                        listener.onMessage(wrapper, String(payload, Charsets.UTF_8))
                    } else {
                        listener.onMessage(wrapper, payload.toByteString())
                    }
                } catch (e: Exception) {
                    Log.w("E2eeWS", "Failed to decrypt WS frame: ${e.message}")
                    webSocket.close(1008, "e2ee_decrypt_failed")
                }
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                // E2EE WS strictly carries encrypted binary frames; ignore unexpected plaintext text frames.
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                listener.onClosing(wrapper, code, reason)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                listener.onClosed(wrapper, code, reason)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                listener.onFailure(wrapper, t, response)
            }
        })
        wrapper = E2eeWebSocketWrapper(rawWs, request, deviceId, key)
        return wrapper
    }

    private class E2eeWebSocketWrapper(
        private val delegate: WebSocket,
        private val originalRequest: Request,
        private val deviceId: String,
        private val key: ByteArray
    ) : WebSocket {
        override fun request(): Request = originalRequest
        override fun queueSize(): Long = delegate.queueSize()
        override fun cancel() = delegate.cancel()
        override fun close(code: Int, reason: String?): Boolean = delegate.close(code, reason)

        override fun send(text: String): Boolean {
            val payload = text.toByteArray(Charsets.UTF_8)
            val plain = ByteArray(1 + payload.size)
            plain[0] = 1 // websocket.TextMessage
            System.arraycopy(payload, 0, plain, 1, payload.size)
            val enc = E2eeCrypto.encrypt(key, plain, E2eeCrypto.frameAad(deviceId, "ws-c2s"))
            return delegate.send(enc.toByteString())
        }

        override fun send(bytes: ByteString): Boolean {
            val payload = bytes.toByteArray()
            val plain = ByteArray(1 + payload.size)
            plain[0] = 2 // websocket.BinaryMessage
            System.arraycopy(payload, 0, plain, 1, payload.size)
            val enc = E2eeCrypto.encrypt(key, plain, E2eeCrypto.frameAad(deviceId, "ws-c2s"))
            return delegate.send(enc.toByteString())
        }
    }
}
