package com.antigravity.mobile.ui.components

import android.graphics.Bitmap
import androidx.compose.animation.core.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.ColorMatrix
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.delay
import java.text.SimpleDateFormat
import java.util.Locale
import java.util.TimeZone
import java.util.concurrent.ConcurrentHashMap
import kotlin.math.*

// MARK: - Swarm Theme (Zero Magenta / Purple)

object SwarmColors {
    fun bg(isDark: Boolean) = if (isDark) Color(0xFF151518) else Color(0xFFFAF8F1)
    fun cardBg(isDark: Boolean) = if (isDark) Color(0xFF1F1F25) else Color(0xFFF2EFE6)
    fun surfaceBg(isDark: Boolean) = if (isDark) Color(0xFF26262D) else Color(0xFFFFFEFB)
    fun fieldBg(isDark: Boolean) = if (isDark) Color(0xFF25252C) else Color(0xFFEFECE3)
    fun border(isDark: Boolean) = if (isDark) Color(0xFF33333C) else Color(0xFFE1DDD1)
    fun ink(isDark: Boolean) = if (isDark) Color(0xFFF3F1EB) else Color(0xFF202124)
    fun muted(isDark: Boolean) = if (isDark) Color(0xFF9E9B94) else Color(0xFF6E6B65)
    fun faint(isDark: Boolean) = if (isDark) Color(0xFF706E69) else Color(0xFF9D9A92)
    fun accent(isDark: Boolean) = if (isDark) Color(0xFFE0864B) else Color(0xFFB85A24) // warm terracotta
    fun liveGreen(isDark: Boolean) = if (isDark) Color(0xFF5EC269) else Color(0xFF3E9E4B)
    fun warnAmber(isDark: Boolean) = if (isDark) Color(0xFFE0A834) else Color(0xFFBE8B1E)
    fun errorRed(isDark: Boolean) = if (isDark) Color(0xFFE06060) else Color(0xFFC93B3B)
    fun infoBlue(isDark: Boolean) = if (isDark) Color(0xFF5EA6E6) else Color(0xFF2871B0)
    fun codeBg(isDark: Boolean) = if (isDark) Color(0xFF2E2E36) else Color(0xFFE9E5DA)
    val coordHexBg = Color(0xFF1C1B22)
}

enum class SwarmBeePose {
    WORKING, ASLEEP, LANDED
}

@Composable
fun SwarmPixelBee(
    name: String,
    pose: SwarmBeePose,
    modifier: Modifier = Modifier,
    size: Dp = 32.dp,
    statusDotColor: Color? = null
) {
    val isDark = AntigravityTheme.colors.isDark
    var frame by remember { mutableIntStateOf(0) }

    LaunchedEffect(pose) {
        if (pose == SwarmBeePose.WORKING) {
            while (true) {
                delay(140)
                frame = if (frame == 0) 1 else 0
            }
        } else {
            frame = 0
        }
    }

    val bitmap = remember(name, pose, frame) {
        PixelBeeBitmapCache.getBitmap(name, pose, if (pose == SwarmBeePose.WORKING) frame else 0)
    }

    Box(
        modifier = modifier.size(size),
        contentAlignment = Alignment.BottomEnd
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .clip(RoundedCornerShape(size * 0.24f))
                .background(
                    if (name.equals("coordinator", ignoreCase = true)) SwarmColors.coordHexBg
                    else PixelBeeBitmapCache.getTileColor(name, isDark)
                )
                .border(
                    width = 0.6.dp,
                    color = if (name.equals("coordinator", ignoreCase = true)) Color.White.copy(alpha = 0.10f)
                    else SwarmColors.border(isDark).copy(alpha = 0.8f),
                    shape = RoundedCornerShape(size * 0.24f)
                ),
            contentAlignment = Alignment.Center
        ) {
            if (name.equals("coordinator", ignoreCase = true)) {
                Canvas(modifier = Modifier.fillMaxSize()) {
                    val r = 8f * density
                    val h = (r * sqrt(3.0) / 2.0).toFloat()
                    val cols = (this.size.width / (r * 1.5f)).toInt() + 2
                    val rows = (this.size.height / h).toInt() + 2
                    for (row in -1..rows) {
                        for (col in -1..cols) {
                            val cx = col * r * 1.5f
                            val cy = row * h * 2f + (if (col % 2 == 0) 0f else h)
                            val path = Path()
                            for (i in 0 until 6) {
                                val angle = (i * PI / 3.0).toFloat()
                                val pt = Offset(cx + r * cos(angle), cy + r * sin(angle))
                                if (i == 0) path.moveTo(pt.x, pt.y) else path.lineTo(pt.x, pt.y)
                            }
                            path.close()
                            drawPath(path, color = Color.White.copy(alpha = 0.08f), style = Stroke(width = 0.7f * density))
                        }
                    }
                }
            }

            Image(
                bitmap = bitmap.asImageBitmap(),
                contentDescription = null,
                filterQuality = FilterQuality.None,
                modifier = Modifier
                    .fillMaxWidth(0.76f)
                    .fillMaxHeight(0.62f)
            )
        }

        if (statusDotColor != null) {
            Box(
                modifier = Modifier
                    .size(maxOf(7.dp, size * 0.25f))
                    .clip(CircleShape)
                    .background(statusDotColor)
                    .border(1.5.dp, SwarmColors.bg(isDark), CircleShape)
                    .offset(x = 2.dp, y = 2.dp)
            )
        }
    }
}

@Composable
fun SwarmPixelBeeSprite(
    name: String,
    pose: SwarmBeePose,
    modifier: Modifier = Modifier,
    width: Dp = 38.dp,
    height: Dp = 31.dp,
    statusDotColor: Color? = null,
    dotBorderColor: Color? = null,
    isTilted: Boolean = false,
    isRetired: Boolean = false,
    bobbing: Boolean = false,
    contentDescription: String? = null
) {
    val isDark = AntigravityTheme.colors.isDark
    var frame by remember { mutableIntStateOf(0) }

    LaunchedEffect(pose) {
        if (pose == SwarmBeePose.WORKING) {
            while (true) {
                delay(140)
                frame = if (frame == 0) 1 else 0
            }
        } else {
            frame = 0
        }
    }

    val bobOffset = if (bobbing) {
        val infinite = rememberInfiniteTransition(label = "beeBob")
        val v by infinite.animateFloat(
            initialValue = 0f,
            targetValue = -2.2f,
            animationSpec = infiniteRepeatable(
                animation = tween(950, easing = FastOutSlowInEasing),
                repeatMode = RepeatMode.Reverse
            ),
            label = "beeBobY"
        )
        v
    } else 0f

    val bitmap = remember(name, pose, frame) {
        PixelBeeBitmapCache.getBitmap(name, pose, if (pose == SwarmBeePose.WORKING) frame else 0)
    }
    val grayscaleFilter = remember(isRetired) {
        if (isRetired) ColorFilter.colorMatrix(ColorMatrix().apply { setToSaturation(0f) }) else null
    }

    Box(
        modifier = modifier.size(width, height),
        contentAlignment = Alignment.TopEnd
    ) {
        Image(
            bitmap = bitmap.asImageBitmap(),
            contentDescription = contentDescription,
            filterQuality = FilterQuality.None,
            colorFilter = grayscaleFilter,
            modifier = Modifier
                .fillMaxSize()
                .offset(y = bobOffset.dp)
                .rotate(if (isTilted) -14f else 0f)
                .alpha(if (isRetired) 0.36f else 1f)
        )

        if (statusDotColor != null) {
            val dotSize = if (width < 26.dp) 6.5.dp else 8.5.dp
            Box(
                modifier = Modifier
                    .size(dotSize)
                    .offset(x = 1.dp, y = (-1).dp)
                    .clip(CircleShape)
                    .background(statusDotColor)
                    .border(1.5.dp, dotBorderColor ?: SwarmColors.surfaceBg(isDark), CircleShape)
            )
        }
    }
}

@Composable
fun SwarmMiniTrioTile(
    names: List<String>,
    size: Dp = 44.dp,
    isActive: Boolean = true,
    modifier: Modifier = Modifier
) {
    val isDark = AntigravityTheme.colors.isDark
    val trio = (names + listOf("worker-a", "worker-b", "worker-c")).take(3)
    val pose = if (isActive) SwarmBeePose.WORKING else SwarmBeePose.LANDED
    Box(
        modifier = modifier
            .size(size)
            .clip(RoundedCornerShape(size * 0.22f))
            .background(SwarmColors.fieldBg(isDark))
    ) {
        SwarmPixelBeeSprite(
            name = trio[0],
            pose = pose,
            width = 20.dp,
            height = 16.dp,
            modifier = Modifier.offset(x = 3.dp, y = 7.dp)
        )
        SwarmPixelBeeSprite(
            name = trio[1],
            pose = pose,
            width = 20.dp,
            height = 16.dp,
            modifier = Modifier.offset(x = 19.dp, y = 4.dp)
        )
        SwarmPixelBeeSprite(
            name = trio[2],
            pose = pose,
            width = 20.dp,
            height = 16.dp,
            modifier = Modifier.offset(x = 11.dp, y = 22.dp)
        )
    }
}

private object PixelBeeBitmapCache {
    private val cache = ConcurrentHashMap<String, Bitmap>()
    // Exact desktop hues filtered to eliminate magenta/purple (330/262/290 shifted to amber/cyan/blue)
    private val paletteHues = doubleArrayOf(12.0, 150.0, 206.0, 40.0, 196.0, 228.0, 96.0, 28.0, 165.0, 175.0, 350.0, 60.0)

    private fun nameHash(s: String): Long {
        var h = 7L
        for (c in s) {
            h = (h * 31L + c.code.toLong()) and 0xFFFFFFFFL
        }
        return h
    }

    fun getHue(name: String): Double {
        if (name.equals("coordinator", ignoreCase = true)) return 46.0
        val idx = (nameHash(name) % paletteHues.size.toLong()).toInt()
        return paletteHues[idx]
    }

    fun getTileColor(name: String, isDark: Boolean): Color {
        val h = getHue(name)
        val rgb = hslToArgb(h, 78.0, 60.0)
        val alpha = if (isDark) 0.24f else 0.20f
        return Color(rgb).copy(alpha = alpha)
    }

    fun getBitmap(name: String, pose: SwarmBeePose, frame: Int): Bitmap {
        val f = if (pose == SwarmBeePose.WORKING) (frame % 2) else 0
        val key = "$name|${pose.name}|$f"
        cache[key]?.let { return it }

        val bmp = renderBee(name, pose, f)
        cache[key] = bmp
        return bmp
    }

    private fun shiftHue(h: Double, target: Double, amount: Double): Double {
        val d = ((target - h + 540.0) % 360.0) - 180.0
        return h + d * amount
    }

    private fun hslToArgb(hRaw: Double, sRaw: Double, lRaw: Double, a: Int = 255): Int {
        val h = ((hRaw % 360.0) + 360.0) % 360.0
        val s = sRaw / 100.0
        val l = lRaw / 100.0
        val amp = s * min(l, 1.0 - l)
        fun comp(n: Double): Int {
            val k = (n + h / 30.0) % 12.0
            val v = l - amp * max(-1.0, min(k - 3.0, min(9.0 - k, 1.0)))
            return max(0, min(255, (v * 255.0).roundToInt()))
        }
        val r = comp(0.0)
        val g = comp(8.0)
        val b = comp(4.0)
        return (a and 0xFF shl 24) or (r and 0xFF shl 16) or (g and 0xFF shl 8) or (b and 0xFF)
    }

    private class Ramp(
        val spec: Int, val hi: Int, val mid: Int, val sh: Int, val ol: Int,
        val skh: Int, val sk: Int, val skd: Int
    )

    private fun createRamp(name: String): Ramp {
        val h = getHue(name)
        return Ramp(
            spec = hslToArgb(shiftHue(h, 55.0, 0.28), 96.0, 90.0),
            hi = hslToArgb(shiftHue(h, 55.0, 0.18), 90.0, 74.0),
            mid = hslToArgb(h, 78.0, 60.0),
            sh = hslToArgb(shiftHue(h, 220.0, 0.22), 58.0, 44.0),
            ol = hslToArgb(shiftHue(h, 220.0, 0.35), 42.0, 19.0),
            skh = hslToArgb(shiftHue(h, 220.0, 0.15), 20.0, 30.0),
            sk = hslToArgb(shiftHue(h, 220.0, 0.30), 24.0, 18.0),
            skd = hslToArgb(shiftHue(h, 220.0, 0.45), 32.0, 10.0)
        )
    }

    private fun renderBee(name: String, pose: SwarmBeePose, frame: Int): Bitmap {
        val w = 22
        val h = 18
        val pixels = IntArray(w * h)

        fun setPixel(x: Int, y: Int, color: Int) {
            if (x !in 0 until w || y !in 0 until h) return
            val sa = (color ushr 24) and 0xFF
            if (sa == 255) {
                pixels[y * w + x] = color
            } else if (sa > 0) {
                val dst = pixels[y * w + x]
                val da = (dst ushr 24) and 0xFF
                val sAlpha = sa / 255.0
                val dAlpha = da / 255.0
                val outA = sAlpha + dAlpha * (1.0 - sAlpha)
                if (outA <= 0.0) return
                val sr = (color ushr 16) and 0xFF
                val sg = (color ushr 8) and 0xFF
                val sb = color and 0xFF
                val dr = (dst ushr 16) and 0xFF
                val dg = (dst ushr 8) and 0xFF
                val db = dst and 0xFF

                val outR = ((sr * sAlpha + dr * dAlpha * (1.0 - sAlpha)) / outA).roundToInt().coerceIn(0, 255)
                val outG = ((sg * sAlpha + dg * dAlpha * (1.0 - sAlpha)) / outA).roundToInt().coerceIn(0, 255)
                val outB = ((sb * sAlpha + db * dAlpha * (1.0 - sAlpha)) / outA).roundToInt().coerceIn(0, 255)
                val outAlpha = (outA * 255.0).roundToInt().coerceIn(0, 255)

                pixels[y * w + x] = (outAlpha shl 24) or (outR shl 16) or (outG shl 8) or outB
            }
        }

        val r = createRamp(name)
        val isQueen = name.equals("coordinator", ignoreCase = true)
        val landed = (pose == SwarmBeePose.LANDED || pose == SwarmBeePose.ASLEEP)
        val asleep = (pose == SwarmBeePose.ASLEEP)

        val cx = 11.0
        val cy = 10.5
        val rx = 7.3
        val ry = 4.8

        fun inside(x: Int, y: Int): Boolean {
            val nx = (x.toDouble() + 0.5 - cx) / rx
            val ny = (y.toDouble() + 0.5 - cy) / ry
            return nx * nx + ny * ny <= 1.0
        }

        val rawL = doubleArrayOf(-0.42, -0.62, 0.66)
        val lenL = sqrt(rawL[0] * rawL[0] + rawL[1] * rawL[1] + rawL[2] * rawL[2])
        val lx = rawL[0] / lenL
        val ly = rawL[1] / lenL
        val lz = rawL[2] / lenL

        val n4 = arrayOf(1 to 0, -1 to 0, 0 to 1, 0 to -1)

        fun rows(spec: List<Triple<Int, Int, Int>>): List<Pair<Int, Int>> {
            val pts = mutableListOf<Pair<Int, Int>>()
            for (t in spec) {
                for (x in t.second..t.third) {
                    pts.add(x to t.first)
                }
            }
            return pts
        }

        fun drawWing(pts: List<Pair<Int, Int>>, glint: Boolean) {
            val set = pts.map { "${it.first},${it.second}" }.toSet()
            val edgeCol = (230 shl 24) or (140 shl 16) or (160 shl 8) or 192
            val fillCol = (184 shl 24) or (255 shl 16) or (255 shl 8) or 255
            for ((x, y) in pts) {
                val isEdge = n4.any { "${x + it.first},${y + it.second}" !in set }
                setPixel(x, y, if (isEdge) edgeCol else fillCol)
            }
            if (glint) {
                val interior = pts.filter { (x, y) ->
                    n4.none { "${x + it.first},${y + it.second}" !in set }
                }.sortedWith(compareBy({ it.second }, { it.first }))
                interior.firstOrNull()?.let {
                    setPixel(it.first, it.second, (255 shl 24) or (255 shl 16) or (255 shl 8) or 255)
                }
            }
        }

        val backWing: List<Pair<Int, Int>>?
        val frontWing: List<Pair<Int, Int>>
        if (landed) {
            backWing = null
            frontWing = rows(listOf(Triple(5, 14, 17), Triple(6, 13, 18)))
        } else if (frame == 0) {
            backWing = rows(listOf(Triple(1, 16, 17), Triple(2, 16, 18), Triple(3, 15, 18), Triple(4, 15, 17), Triple(5, 15, 16)))
            frontWing = rows(listOf(Triple(0, 13, 14), Triple(1, 12, 15), Triple(2, 11, 15), Triple(3, 11, 14), Triple(4, 11, 13), Triple(5, 11, 12)))
        } else {
            backWing = rows(listOf(Triple(3, 16, 18), Triple(4, 15, 19), Triple(5, 16, 18)))
            frontWing = rows(listOf(Triple(3, 13, 16), Triple(4, 12, 17), Triple(5, 12, 17), Triple(6, 13, 15)))
        }

        if (backWing != null) {
            drawWing(backWing, glint = false)
        }

        fun stripe(x: Int, y: Int): Boolean {
            val ny = (y.toDouble() + 0.5 - cy) / ry
            val u = x.toDouble() + 0.5 + 1.15 * ny * ny
            return (u > 10.6 && u < 12.5) || (u > 14.3 && u < 16.1) || u > 17.9
        }

        for (y in 0 until h) {
            for (x in 0 until w) {
                if (!inside(x, y)) continue
                val nx = (x.toDouble() + 0.5 - cx) / rx
                val ny = (y.toDouble() + 0.5 - cy) / ry
                val rr = min(1.0, nx * nx + ny * ny)
                val nz = sqrt(1.0 - rr)
                val intensity = nx * lx + ny * ly + nz * lz
                if (stripe(x, y)) {
                    setPixel(x, y, if (intensity > 0.80) r.skh else if (intensity > 0.30) r.sk else r.skd)
                } else {
                    setPixel(x, y, if (intensity > 0.965) r.spec else if (intensity > 0.78) r.hi else if (intensity > 0.26) r.mid else r.sh)
                }
            }
        }

        for (y in 0 until h) {
            for (x in 0 until w) {
                if (inside(x, y)) continue
                val nbs = n4.filter { inside(x + it.first, y + it.second) }
                if (nbs.isEmpty()) continue
                val nx = (x.toDouble() + 0.5 - cx) / rx
                val ny = (y.toDouble() + 0.5 - cy) / ry
                val n = sqrt(nx * nx + ny * ny)
                val d = (nx / n) * lx + (ny / n) * ly
                val nextToStripe = nbs.any { stripe(x + it.first, y + it.second) }
                setPixel(x, y, if (d > 0.5 && !nextToStripe) r.sh else r.ol)
            }
        }

        // Stinger
        setPixel(19, 10, r.ol)
        setPixel(20, 10, r.ol)

        // Face
        if (asleep) {
            setPixel(6, 10, r.ol)
            setPixel(7, 10, r.ol)
        } else {
            setPixel(6, 9, (255 shl 24) or (255 shl 16) or (255 shl 8) or 255)
            setPixel(7, 9, r.skd)
            setPixel(6, 10, r.skd)
            setPixel(7, 10, r.skd)
        }
        // Coral blush
        setPixel(5, 11, (140 shl 24) or (245 shl 16) or (132 shl 8) or 96)
        setPixel(4, 11, (90 shl 24) or (245 shl 16) or (132 shl 8) or 96)

        // Antennae / Queen Crown
        if (isQueen) {
            val g1 = (255 shl 24) or (247 shl 16) or (206 shl 8) or 91
            val g2 = (255 shl 24) or (255 shl 16) or (233 shl 8) or 166
            val g3 = (255 shl 24) or (201 shl 16) or (138 shl 8) or 43
            val gem = (255 shl 24) or (228 shl 16) or (100 shl 8) or 62
            for ((cx2, cy2) in listOf(5 to 5, 6 to 5, 7 to 5, 8 to 5, 9 to 5)) { setPixel(cx2, cy2, g1) }
            setPixel(5, 4, g2)
            setPixel(7, 4, g2)
            setPixel(9, 4, g2)
            setPixel(7, 3, g2)
            setPixel(5, 5, g3)
            setPixel(9, 5, g3)
            setPixel(7, 5, gem)
        } else {
            for ((cx2, cy2) in listOf(7 to 5, 6 to 4, 5 to 3)) { setPixel(cx2, cy2, r.ol) }
            setPixel(4, 3, r.sk)
            for ((cx2, cy2) in listOf(9 to 5, 9 to 4, 9 to 3)) { setPixel(cx2, cy2, r.ol) }
            setPixel(8, 2, r.sk)
        }

        drawWing(frontWing, glint = true)

        if (asleep) {
            val zCol = (255 shl 24) or (140 shl 16) or (144 shl 8) or 166
            for ((zx, zy) in listOf(0 to 0, 1 to 0, 2 to 0, 3 to 0, 2 to 1, 1 to 2, 0 to 3, 1 to 3, 2 to 3, 3 to 3)) {
                setPixel(zx, zy, zCol)
            }
        }

        return Bitmap.createBitmap(pixels, w, h, Bitmap.Config.ARGB_8888)
    }
}

// MARK: - Swarm Hive & Overview Synthesis Engine (Ported from swarm_board/hive.js)

enum class SwarmTaskBucketState { DONE, RUNNING, BLOCKED, QUEUED }
enum class SwarmThreadState { NEEDS, WAITING, STUCK, RUNNING, QUEUED, DONE }
enum class SwarmHiveBeeState { WORKING, WAIT, NEEDS, ISSUE, DONE, IDLE, RETIRED, UNBORN }

data class SwarmSynthesizedMember(
    val name: String,
    val conversationId: String,
    val role: String,
    val title: String,
    val model: String,
    val status: String, // "needs_you", "working", "waiting_on", "parked"
    val waitingOn: String?,
    val now: String,
    val liveText: String?,
    val liveKind: String?,
    val hasIssue: Boolean,
    val killed: Boolean,
    val posts: Int,
    val lastPostSeq: Int
)

data class SwarmSynthesizedTask(
    val id: String,
    val title: String,
    val owners: List<String>,
    val state: SwarmTaskBucketState,
    val rawState: String,
    val note: String,
    val liveActivity: String?,
    val waitingOn: String?,
    val seq: Int
)

data class SwarmSynthesizedThread(
    val id: String,
    val title: String,
    val goal: String,
    val now: String,
    val state: SwarmThreadState,
    val statusLabel: String,
    val tasks: List<SwarmSynthesizedTask>,
    val doneCount: Int,
    val runningCount: Int,
    val blockedCount: Int,
    val queuedCount: Int
) {
    val totalCount: Int get() = doneCount + runningCount + blockedCount + queuedCount
}

data class SwarmSynthesizedModel(
    val goalTitle: String,
    val goalLine: String,
    val runningSummary: String,
    val phaseLabel: String?,
    val finished: Boolean,
    val stallSeq: Int?,
    val stallAgent: String?,
    val stallText: String?,
    val members: Map<String, SwarmSynthesizedMember>,
    val orderedCrew: List<SwarmSynthesizedMember>,
    val tasks: List<SwarmSynthesizedTask>,
    val threads: List<SwarmSynthesizedThread>,
    val doneCount: Int,
    val runningCount: Int,
    val blockedCount: Int,
    val queuedCount: Int,
    val reviewCount: Int,
    val openThreadsCount: Int,
    val needsYouCount: Int,
    val stuckCount: Int,
    val waitingChips: List<String>
) {
    val totalTasksCount: Int get() = doneCount + runningCount + blockedCount + queuedCount
}

data class SwarmHiveCell(
    val id: String,
    val title: String,
    val goal: String,
    val bees: List<String>,
    val done: Int,
    val total: Int,
    val countOnly: Int?,
    val q: Int,
    val r: Int
)

data class SwarmHiveMapModel(
    val cells: List<SwarmHiveCell>,
    val beeStates: Map<String, SwarmHiveBeeState>,
    val beeWaitingOn: Map<String, String>,
    val allDone: Int,
    val allTotal: Int,
    val coordinatorNeedsYou: Boolean,
    val beamSeq: Int?,
    val beamFrom: String?,
    val beamTargets: List<String>,
    val waitLinks: List<Pair<String, String>>
)

object SwarmHiveEngine {
    private val askQuestionRegex = Regex("""\?|could you|can you|please|ping me|waiting on|请|能否""", RegexOption.IGNORE_CASE)
    private val doneTagRegex = Regex("""^(?:@[A-Za-z0-9_\-]+[\s,:–—\-]*)*(?:\*\*|\*|`|_)?\[done\]""", RegexOption.IGNORE_CASE)

    fun parseSwarmTimeMs(raw: String?): Long? {
        val s = raw?.trim().orEmpty()
        if (s.isEmpty()) return null
        val patterns = listOf(
            "yyyy-MM-dd HH:mm:ss" to false,
            "yyyy-MM-dd'T'HH:mm:ss'Z'" to true,
            "yyyy-MM-dd'T'HH:mm:ss.SSS'Z'" to true,
            "yyyy-MM-dd'T'HH:mm:ssXXX" to false
        )
        for ((pat, utc) in patterns) {
            try {
                val fmt = SimpleDateFormat(pat, Locale.US)
                if (utc) fmt.timeZone = TimeZone.getTimeZone("UTC")
                val d = fmt.parse(s)
                if (d != null) return d.time
            } catch (_: Exception) {}
        }
        return null
    }

    fun formatSpanDuration(startRaw: String?, endRaw: String?): String? {
        val t1 = parseSwarmTimeMs(startRaw) ?: return null
        val t2 = parseSwarmTimeMs(endRaw) ?: return null
        val sec = ((t2 - t1) / 1000L).coerceAtLeast(0L)
        if (sec <= 0L) return null
        if (sec < 60L) return "${sec}s"
        val mins = sec / 60L
        val rem = sec % 60L
        if (mins < 60L) return if (rem > 0L) "${mins}m ${rem}s" else "${mins}m"
        val hrs = mins / 60L
        val remMins = mins % 60L
        return if (remMins > 0L) "${hrs}h ${remMins}m" else "${hrs}h"
    }

    private fun isDonePost(text: String): Boolean = doneTagRegex.containsMatchIn(text.trim())
    private fun looksLikeQuestion(text: String): Boolean = askQuestionRegex.containsMatchIn(text)

    private fun cleanInlineText(raw: String, maxChars: Int = 180): String {
        val s = raw.replace("**", "").replace("`", "").replace(Regex("\\s+"), " ").trim()
        return if (s.length > maxChars) s.take(maxChars - 1).trimEnd() + "…" else s
    }

    private fun extractRoleHint(memberName: String, role: String, messages: List<SwarmBoardMessage>): String {
        if (memberName.equals("coordinator", ignoreCase = true) || role.equals("coordinator", ignoreCase = true)) {
            return "协调者".loc
        }
        val brief = messages.firstOrNull { m ->
            m.agent.equals("coordinator", ignoreCase = true) &&
                m.mentions.any { it.equals(memberName, ignoreCase = true) }
        }
        if (brief != null) {
            var body = stripLeadingMentions(brief.text).lineSequence().firstOrNull().orEmpty().trim()
            for (prefix in listOf("You own ", "You are the ", "You are this swarm's ", "You are ")) {
                if (body.startsWith(prefix)) {
                    body = body.removePrefix(prefix)
                    break
                }
            }
            body = body.trim().trimEnd('.', ':')
            if (body.isNotEmpty()) return body
        }
        return role.ifBlank { "worker" }
    }

    fun synthesize(snap: SwarmSnapshot, filterMember: String? = null): SwarmSynthesizedModel {
        val leadMember = snap.members.firstOrNull { it.role.equals("coordinator", ignoreCase = true) || it.name.equals("coordinator", ignoreCase = true) }
        val leadName = leadMember?.name ?: "coordinator"
        fun normKey(n: String): String = if (n == leadName) "coordinator" else n

        val teamMembers = snap.members.filter { !it.roleWorker && !isRoleWorker(it.name, it.role) }
        val byName = snap.members.associateBy { it.name }
        val teamMsgs = snap.messages.filter { m ->
            if (m.text.startsWith("[channel]")) return@filter false
            val info = byName[m.agent]
            if (info != null) !info.roleWorker && !isRoleWorker(info.name, info.role)
            else !isRoleWorker(m.agent, "")
        }

        val headSeq = maxOf(snap.brief.latestSeq, teamMsgs.lastOrNull()?.seq ?: 0)
        val hasLiveWorking = teamMembers.any { !it.killed && it.status.equals("working", ignoreCase = true) }
        val hasSystemError = teamMembers.any { !it.killed && (it.status.equals("error", ignoreCase = true) || it.status.equals("stuck", ignoreCase = true)) }
        val lastPost = teamMsgs.lastOrNull()
        val finished = !hasLiveWorking && !hasSystemError &&
            (lastPost != null && normKey(lastPost.agent) == "coordinator" && isDonePost(lastPost.text))

        val narratorView = snap.view
        val planTasks = snap.plan?.tasks ?: emptyMap()

        fun lastPostBy(key: String): SwarmBoardMessage? = teamMsgs.lastOrNull { normKey(it.agent) == key }

        val synthMembers = mutableMapOf<String, SwarmSynthesizedMember>()
        for (m in teamMembers) {
            val k = normKey(m.name)
            val lp = lastPostBy(k)
            val rawSt = m.status.lowercase()
            val hasIssue = !m.killed && !finished && (rawSt == "error" || rawSt == "stuck")
            var st = if (m.killed) "parked" else if (rawSt == "working") "working" else "parked"
            var waitingOn: String? = null

            if (st == "parked" && !finished && !m.killed && lp != null) {
                val directTargets = extractTargets(lp)
                    .filter { !it.equals("all", ignoreCase = true) && !isRoleWorker(it, "") }
                    .map(::normKey)
                val firstTarget = directTargets.firstOrNull()
                if (firstTarget != null && looksLikeQuestion(lp.text)) {
                    val answered = teamMsgs.any { it.seq > lp.seq && normKey(it.agent) == firstTarget }
                    if (!answered) {
                        st = "waiting_on"
                        waitingOn = firstTarget
                    }
                }
            }

            val actTrimmed = m.lastActivity?.trim().orEmpty()
            val isGenericWait = actTrimmed.contains("swarm_wait") || actTrimmed == "等待新消息" || actTrimmed == "Waiting for posts"
            val effectiveLive = if (rawSt == "working" && !m.killed && !hasIssue && actTrimmed.isNotEmpty() && !isGenericWait) {
                cleanInlineText(actTrimmed, 120)
            } else null

            val nowLine = when {
                effectiveLive != null -> effectiveLive
                !narratorView?.now?.get(m.name).isNullOrBlank() -> cleanInlineText(narratorView!!.now[m.name]!!, 120)
                lp != null -> cleanInlineText(stripLeadingMentions(lp.text), 140)
                else -> ""
            }

            val roleTitle = when {
                k == "coordinator" -> "协调者".loc
                !planTasks[m.name].isNullOrBlank() -> planTasks[m.name]!!.trim()
                else -> extractRoleHint(m.name, m.role, snap.messages)
            }

            val postCount = if (m.posts > 0) m.posts else teamMsgs.count { normKey(it.agent) == k }
            val lastSeq = if (m.lastPostSeq > 0) m.lastPostSeq else (lp?.seq ?: 0)

            synthMembers[k] = SwarmSynthesizedMember(
                name = k,
                conversationId = m.conversationId,
                role = m.role,
                title = roleTitle,
                model = m.model,
                status = st,
                waitingOn = waitingOn,
                now = nowLine,
                liveText = effectiveLive,
                liveKind = m.activityKind,
                hasIssue = hasIssue,
                killed = m.killed,
                posts = postCount,
                lastPostSeq = lastSeq
            )
        }

        if (!synthMembers.containsKey("coordinator")) {
            synthMembers["coordinator"] = SwarmSynthesizedMember(
                name = "coordinator",
                conversationId = snap.coordinatorId,
                role = "coordinator",
                title = "协调者".loc,
                model = "",
                status = "parked",
                waitingOn = null,
                now = lastPostBy("coordinator")?.let { cleanInlineText(stripLeadingMentions(it.text), 140) }.orEmpty(),
                liveText = null,
                liveKind = null,
                hasIssue = false,
                killed = false,
                posts = 0,
                lastPostSeq = 0
            )
        }

        val allTasks = mutableListOf<SwarmSynthesizedTask>()
        val vws = narratorView?.workstreams.orEmpty()
        if (vws.isNotEmpty()) {
            for (w in vws) {
                val owners = if (w.owners.isEmpty()) listOf("coordinator") else w.owners.map(::normKey)
                val raw0 = w.state.lowercase()
                val rawState = if (finished && raw0 != "failed") "done" else raw0
                var bucket = when (rawState) {
                    "done" -> SwarmTaskBucketState.DONE
                    "active", "review" -> SwarmTaskBucketState.RUNNING
                    "blocked", "failed" -> SwarmTaskBucketState.BLOCKED
                    else -> SwarmTaskBucketState.QUEUED
                }
                var waitTarget: String? = null
                if (bucket != SwarmTaskBucketState.DONE && bucket != SwarmTaskBucketState.BLOCKED) {
                    val om = synthMembers[owners.first()]
                    if (om?.status == "waiting_on") {
                        bucket = SwarmTaskBucketState.BLOCKED
                        waitTarget = om.waitingOn
                    }
                }
                val liveOwner = if (bucket == SwarmTaskBucketState.RUNNING) {
                    owners.mapNotNull { synthMembers[it] }.firstOrNull { !it.killed && it.liveText != null }
                } else null
                allTasks.add(
                    SwarmSynthesizedTask(
                        id = w.id,
                        title = w.title,
                        owners = owners,
                        state = bucket,
                        rawState = rawState,
                        note = cleanInlineText(w.note ?: liveOwner?.now.orEmpty()),
                        liveActivity = liveOwner?.liveText,
                        waitingOn = waitTarget,
                        seq = headSeq
                    )
                )
            }
        } else {
            for (m in teamMembers.filter { !it.role.equals("coordinator", ignoreCase = true) && it.name != leadName }) {
                val k = normKey(m.name)
                val sm = synthMembers[k] ?: continue
                val lp = lastPostBy(k)
                val rawState = when {
                    sm.killed -> "done"
                    sm.status == "waiting_on" -> "blocked"
                    sm.status == "working" || sm.status == "needs_you" -> "active"
                    lp != null -> if (finished || isDonePost(lp.text)) "done" else "review"
                    else -> "queued"
                }
                val bucket = when (rawState) {
                    "done" -> SwarmTaskBucketState.DONE
                    "active", "review" -> SwarmTaskBucketState.RUNNING
                    "blocked" -> SwarmTaskBucketState.BLOCKED
                    else -> SwarmTaskBucketState.QUEUED
                }
                val taskTitle = planTasks[m.name]?.trim()?.takeIf { it.isNotEmpty() } ?: sm.title.ifBlank { k }
                allTasks.add(
                    SwarmSynthesizedTask(
                        id = k,
                        title = taskTitle,
                        owners = listOf(k),
                        state = bucket,
                        rawState = rawState,
                        note = sm.now,
                        liveActivity = sm.liveText,
                        waitingOn = sm.waitingOn,
                        seq = lp?.seq ?: headSeq
                    )
                )
            }
        }

        val taskById = allTasks.associateBy { it.id }
        val activeFilter = filterMember?.takeIf { it.isNotBlank() }
        fun matchesFilter(t: SwarmSynthesizedTask): Boolean = activeFilter == null || t.owners.contains(activeFilter)

        data class RawSpec(val id: String, val title: String, val goal: String, val now: String, val taskIds: List<String>)
        val rawSpecs: List<RawSpec> = when {
            !narratorView?.threads.isNullOrEmpty() -> narratorView!!.threads.map {
                RawSpec(it.id.ifBlank { it.title }, it.title, it.goal, it.now, it.workstreams.map(::normKey))
            }
            !snap.plan?.threads.isNullOrEmpty() -> snap.plan!!.threads.map {
                RawSpec(it.id.ifBlank { it.title }, it.title, it.goal, "", it.tasks.map(::normKey))
            }
            else -> emptyList()
        }

        fun buildThread(id: String, title: String, goal: String, explicitNow: String, items: List<SwarmSynthesizedTask>): SwarmSynthesizedThread? {
            if (items.isEmpty()) return null
            var d = 0; var r = 0; var b = 0; var q = 0
            val waiters = mutableSetOf<String>()
            for (item in items) {
                when (item.state) {
                    SwarmTaskBucketState.DONE -> d++
                    SwarmTaskBucketState.RUNNING -> r++
                    SwarmTaskBucketState.BLOCKED -> { b++; item.waitingOn?.let { waiters.add(it) } }
                    SwarmTaskBucketState.QUEUED -> q++
                }
            }
            val hasStuck = items.any { t -> t.owners.any { synthMembers[it]?.hasIssue == true } }
            val tState = when {
                waiters.isNotEmpty() || b > 0 -> SwarmThreadState.WAITING
                hasStuck -> SwarmThreadState.STUCK
                r > 0 -> SwarmThreadState.RUNNING
                q > 0 && d == 0 -> SwarmThreadState.QUEUED
                q > 0 -> SwarmThreadState.RUNNING
                else -> SwarmThreadState.DONE
            }
            val statusLabel = when (tState) {
                SwarmThreadState.NEEDS -> "待你介入".loc
                SwarmThreadState.WAITING -> if (waiters.size == 1) "等待 @%s".loc(waiters.first()) else if (waiters.size > 1) "等待 %d 人".loc(waiters.size) else "等待中".loc
                SwarmThreadState.STUCK -> "卡住".loc
                SwarmThreadState.RUNNING -> "推进中".loc
                SwarmThreadState.QUEUED -> "未开始".loc
                SwarmThreadState.DONE -> "已完成".loc
            }
            val derivedNow = explicitNow.ifBlank {
                items.flatMap { it.owners }.mapNotNull { synthMembers[it] }
                    .firstOrNull { !it.killed && it.status == "working" && it.now.isNotEmpty() }
                    ?.let { it.liveText ?: it.now }.orEmpty()
            }
            val sortedItems = items.sortedBy {
                when (it.state) {
                    SwarmTaskBucketState.BLOCKED -> 0
                    SwarmTaskBucketState.RUNNING -> 1
                    SwarmTaskBucketState.QUEUED -> 2
                    SwarmTaskBucketState.DONE -> 3
                }
            }
            return SwarmSynthesizedThread(id, title, goal, derivedNow, tState, statusLabel, sortedItems, d, r, b, q)
        }

        val usedIds = mutableSetOf<String>()
        val synthThreads = mutableListOf<SwarmSynthesizedThread>()
        for (spec in rawSpecs) {
            usedIds.addAll(spec.taskIds)
            val ws = spec.taskIds.mapNotNull { taskById[it] }.filter(::matchesFilter)
            buildThread(spec.id, spec.title, spec.goal, spec.now, ws)?.let { synthThreads.add(it) }
        }
        val remaining = allTasks.filter { !usedIds.contains(it.id) }.filter(::matchesFilter)
        if (remaining.isNotEmpty() && rawSpecs.isNotEmpty()) {
            buildThread("t-other", "其他".loc, "", "", remaining)?.let { synthThreads.add(it) }
        }
        synthThreads.sortBy { it.state.ordinal }

        val visibleTasks = allTasks.filter(::matchesFilter)
        var totDone = 0; var totRun = 0; var totBlk = 0; var totQue = 0; var totRev = 0
        val waitMap = mutableMapOf<String, Int>()
        for (t in visibleTasks) {
            when (t.state) {
                SwarmTaskBucketState.DONE -> totDone++
                SwarmTaskBucketState.RUNNING -> { totRun++; if (t.rawState == "review") totRev++ }
                SwarmTaskBucketState.BLOCKED -> { totBlk++; t.waitingOn?.let { waitMap[it] = (waitMap[it] ?: 0) + 1 } }
                SwarmTaskBucketState.QUEUED -> totQue++
            }
        }
        val waitingChips = waitMap.entries.sortedByDescending { it.value }.take(2).map { "%1\$d 项等待 @%2\$s".loc(it.value, it.key) }
        val stuckCount = synthMembers.values.count { !it.killed && it.hasIssue }
        val openThreads = synthThreads.count { it.state != SwarmThreadState.DONE }

        val orderedCrew = synthMembers.values
            .filter { it.name != "coordinator" && !it.killed }
            .sortedWith(compareBy<SwarmSynthesizedMember> {
                when {
                    it.status == "needs_you" -> 0
                    it.status == "working" -> 1
                    it.status == "waiting_on" || it.hasIssue -> 2
                    else -> 3
                }
            }.thenBy { it.name })

        var stallSeq: Int? = null; var stallAgent: String? = null; var stallText: String? = null
        if (!finished && !hasLiveWorking) {
            val monMsg = snap.messages.lastOrNull { isRoleWorker(it.agent, "") && it.text.contains("[stall]", ignoreCase = true) }
            if (monMsg != null && monMsg.seq >= (lastPost?.seq ?: 0)) {
                stallSeq = monMsg.seq
                stallAgent = monMsg.agent
                stallText = cleanInlineText(monMsg.text.replace(Regex("\\[stall\\]", RegexOption.IGNORE_CASE), ""), 200)
            }
        }

        val goalObj = narratorView?.goal ?: snap.plan?.goal
        val goalTitle = goalObj?.title?.takeIf { it.isNotBlank() } ?: narratorView?.headline.orEmpty()
        val goalLine = goalObj?.line?.takeIf { it.isNotBlank() } ?: (if (!goalObj?.title.isNullOrBlank()) narratorView?.headline.orEmpty() else "")
        val phaseLabel = narratorView?.phase?.let { p ->
            if (p.total > 1) "${p.label} · ${p.index}/${p.total}" else p.label.takeIf { it.isNotBlank() }
        }

        return SwarmSynthesizedModel(
            goalTitle = goalTitle,
            goalLine = goalLine,
            runningSummary = narratorView?.running_summary.orEmpty(),
            phaseLabel = phaseLabel,
            finished = finished,
            stallSeq = stallSeq,
            stallAgent = stallAgent,
            stallText = stallText,
            members = synthMembers,
            orderedCrew = orderedCrew,
            tasks = visibleTasks,
            threads = synthThreads,
            doneCount = totDone,
            runningCount = totRun,
            blockedCount = totBlk,
            queuedCount = totQue,
            reviewCount = totRev,
            openThreadsCount = openThreads,
            needsYouCount = 0,
            stuckCount = stuckCount,
            waitingChips = waitingChips
        )
    }

    private val hvRing = listOf(0 to -1, 1 to -1, 1 to 0, 0 to 1, -1 to 1, -1 to 0)
    private val hvFew = mapOf(
        1 to listOf(0),
        2 to listOf(5, 1),
        3 to listOf(0, 2, 4),
        4 to listOf(5, 1, 4, 2),
        5 to listOf(5, 0, 1, 4, 2)
    )

    fun hiveSlots(count: Int): List<Pair<Int, Int>> {
        if (count < 1) return emptyList()
        hvFew[count]?.let { preset -> return preset.map { hvRing[it] } }
        val out = mutableListOf<Pair<Int, Int>>()
        val dirs = listOf(1 to 0, 0 to 1, -1 to 1, -1 to 0, 0 to -1, 1 to -1)
        var ring = 1
        while (out.size < count) {
            var q = 0; var r = -ring
            for ((dq, dr) in dirs) {
                repeat(ring) {
                    out.add(q to r)
                    q += dq; r += dr
                }
            }
            ring++
        }
        return out.take(count)
    }

    fun buildHiveMap(snap: SwarmSnapshot, synth: SwarmSynthesizedModel, replaySeq: Int?): SwarmHiveMapModel {
        val leadName = snap.members.firstOrNull { it.role.equals("coordinator", ignoreCase = true) || it.name.equals("coordinator", ignoreCase = true) }?.name ?: "coordinator"
        fun normKey(n: String): String = if (n == leadName) "coordinator" else n

        val teamMsgs = snap.messages.filter { !it.text.startsWith("[channel]") && !isRoleWorker(it.agent, "") }
        val names = synth.members.keys.toList()

        val beeStates = mutableMapOf<String, SwarmHiveBeeState>()
        val beeWaitingOn = mutableMapOf<String, String>()
        var replayDoneCheck: ((String) -> Boolean)? = null
        var beamMsg: SwarmBoardMessage? = null

        if (replaySeq != null) {
            val prefixMsgs = teamMsgs.filter { it.seq <= replaySeq }
            val finIdx = mutableMapOf<String, Int>()
            teamMsgs.forEachIndexed { idx, m -> finIdx[normKey(m.agent)] = idx }
            val born = mutableSetOf("coordinator")
            val lastPostIdx = mutableMapOf<String, Int>()
            val lastInIdx = mutableMapOf<String, Int>()

            prefixMsgs.forEachIndexed { idx, m ->
                val sender = normKey(m.agent)
                born.add(sender)
                lastPostIdx[sender] = idx
                val isBcast = m.mentions.any { it.equals("all", ignoreCase = true) }
                if (!isBcast) {
                    for (t in extractTargets(m).filter { !isRoleWorker(it, "") }) {
                        val tk = normKey(t)
                        born.add(tk)
                        lastInIdx[tk] = idx
                    }
                }
            }
            val curIdx = prefixMsgs.size - 1
            for (n in names) {
                val sm = synth.members[n] ?: continue
                val lp = lastPostIdx[n] ?: -1
                val li = lastInIdx[n] ?: -1
                var st = SwarmHiveBeeState.IDLE
                var who: String? = null

                if (!born.contains(n)) {
                    st = SwarmHiveBeeState.UNBORN
                } else if (sm.killed && finIdx[n] != null && curIdx >= finIdx[n]!! && curIdx > finIdx[n]!! + 8) {
                    st = SwarmHiveBeeState.RETIRED
                } else if (li > lp) {
                    st = SwarmHiveBeeState.WORKING
                    if (li >= 0) who = normKey(prefixMsgs[li].agent)
                } else if (lp >= 0) {
                    val post = prefixMsgs[lp]
                    val isBcast = post.mentions.any { it.equals("all", ignoreCase = true) }
                    val targets = if (isBcast) emptyList() else extractTargets(post).filter { !isRoleWorker(it, "") }.map(::normKey)
                    val firstT = targets.firstOrNull()
                    if (firstT != null && looksLikeQuestion(post.text) && (lastPostIdx[firstT] ?: -1) <= lp) {
                        st = SwarmHiveBeeState.WAIT
                        who = firstT
                    } else if (finIdx[n] != null && finIdx[n]!! <= curIdx && n != "coordinator") {
                        st = SwarmHiveBeeState.DONE
                    } else if (curIdx - lp < 3) {
                        st = SwarmHiveBeeState.WORKING
                    }
                }
                beeStates[n] = st
                if (who != null && who != n) beeWaitingOn[n] = who
            }
            replayDoneCheck = { w ->
                beeStates[w] == SwarmHiveBeeState.RETIRED || (finIdx[w] != null && finIdx[w]!! <= curIdx)
            }
            beamMsg = prefixMsgs.lastOrNull()
        } else {
            for (n in names) {
                val sm = synth.members[n] ?: continue
                val owned = synth.tasks.filter { it.owners.contains(n) }
                val st = when {
                    sm.killed -> SwarmHiveBeeState.RETIRED
                    synth.finished -> if (n == "coordinator") SwarmHiveBeeState.IDLE else SwarmHiveBeeState.DONE
                    sm.hasIssue -> SwarmHiveBeeState.ISSUE
                    sm.status == "working" -> SwarmHiveBeeState.WORKING
                    sm.status == "waiting_on" -> {
                        sm.waitingOn?.let { beeWaitingOn[n] = it }
                        SwarmHiveBeeState.WAIT
                    }
                    n != "coordinator" && owned.isNotEmpty() && owned.all { it.state == SwarmTaskBucketState.DONE } -> SwarmHiveBeeState.DONE
                    else -> SwarmHiveBeeState.IDLE
                }
                beeStates[n] = st
            }
            val lp = teamMsgs.lastOrNull()
            if (lp != null) {
                val ms = parseSwarmTimeMs(lp.time)
                if (ms == null || (System.currentTimeMillis() - ms) < 180_000L) {
                    beamMsg = lp
                }
            }
        }

        val workers = names.filter { it != "coordinator" }.sorted()
        val taskById = synth.tasks.associateBy { it.id }
        fun isTaskDone(t: SwarmSynthesizedTask): Boolean =
            replayDoneCheck?.let { rd -> t.owners.all { it == "coordinator" || rd(it) } } ?: (t.state == SwarmTaskBucketState.DONE)

        val rawThreads: List<Triple<String, Pair<String, String>, List<String>>> = when {
            !snap.view?.threads.isNullOrEmpty() -> snap.view!!.threads.map { Triple(it.id.ifBlank { it.title }, it.title to it.goal, it.workstreams.map(::normKey)) }
            !snap.plan?.threads.isNullOrEmpty() -> snap.plan!!.threads.map { Triple(it.id.ifBlank { it.title }, it.title to it.goal, it.tasks.map(::normKey)) }
            else -> emptyList()
        }

        data class TempCell(
            val id: String, val title: String, val goal: String,
            val ws: List<SwarmSynthesizedTask>, val bees: MutableList<String>,
            val done: Int, val total: Int, val countOnly: Int?
        )

        val tempCells = mutableListOf<TempCell>()
        if (rawThreads.isNotEmpty()) {
            for ((tid, tg, wsIds) in rawThreads) {
                val ws = wsIds.mapNotNull { taskById[it] }
                tempCells.add(TempCell(tid, tg.first, tg.second, ws, mutableListOf(), ws.count(::isTaskDone), ws.size, null))
            }
            val homed = mutableSetOf<String>()
            for (w in workers) {
                val matching = tempCells.filter { c -> c.ws.any { it.owners.contains(w) } }
                if (matching.isEmpty()) continue
                val chosen = matching.firstOrNull { c -> c.ws.any { it.owners.contains(w) && !isTaskDone(it) } } ?: matching.first()
                chosen.bees.add(w)
                homed.add(w)
            }
            val rest = workers.filter { !homed.contains(it) }
            if (rest.isNotEmpty()) {
                val doneRest = rest.count { beeStates[it] == SwarmHiveBeeState.DONE || beeStates[it] == SwarmHiveBeeState.RETIRED }
                tempCells.add(TempCell("_other", "其他".loc, "", emptyList(), rest.toMutableList(), doneRest, rest.size, null))
            }
        } else {
            val buckets = listOf(
                Triple("Working", "工作中".loc, setOf(SwarmHiveBeeState.WORKING)),
                Triple("Waiting", "等待中".loc, setOf(SwarmHiveBeeState.WAIT, SwarmHiveBeeState.ISSUE)),
                Triple("Resting", "休眠中".loc, setOf(SwarmHiveBeeState.IDLE)),
                Triple("Done", "已完成".loc, setOf(SwarmHiveBeeState.DONE, SwarmHiveBeeState.RETIRED))
            )
            for ((key, title, allowed) in buckets) {
                val matched = workers.filter { allowed.contains(beeStates[it] ?: SwarmHiveBeeState.IDLE) }
                if (matched.isNotEmpty()) {
                    tempCells.add(TempCell("_$key", title, "", emptyList(), matched.toMutableList(), if (key == "Done") matched.size else 0, matched.size, matched.size))
                }
            }
        }

        val slots = hiveSlots(tempCells.size)
        val finalCells = tempCells.mapIndexed { idx, tc ->
            val (q, r) = slots.getOrElse(idx) { 0 to 0 }
            SwarmHiveCell(tc.id, tc.title, tc.goal, tc.bees, tc.done, tc.total, tc.countOnly, q, r)
        }

        val allDone = if (replayDoneCheck != null) finalCells.sumOf { it.done } else synth.doneCount
        val allTotal = if (replayDoneCheck != null) finalCells.sumOf { it.total } else maxOf(1, synth.totalTasksCount)

        var beamFrom: String? = null
        var beamTargets = emptyList<String>()
        if (beamMsg != null) {
            beamFrom = normKey(beamMsg.agent)
            val isBcast = beamMsg.mentions.any { it.equals("all", ignoreCase = true) }
            if (!isBcast) {
                beamTargets = extractTargets(beamMsg)
                    .filter { !isRoleWorker(it, "") }
                    .map(::normKey)
                    .filter { beeStates[it] != null && beeStates[it] != SwarmHiveBeeState.UNBORN }
            }
        }

        val waitLinks = beeStates.entries
            .filter { it.value == SwarmHiveBeeState.WAIT }
            .mapNotNull { (n, _) ->
                val w = beeWaitingOn[n] ?: return@mapNotNull null
                if (beeStates[w] != null && beeStates[w] != SwarmHiveBeeState.UNBORN) n to w else null
            }

        return SwarmHiveMapModel(
            cells = finalCells,
            beeStates = beeStates,
            beeWaitingOn = beeWaitingOn,
            allDone = allDone,
            allTotal = allTotal,
            coordinatorNeedsYou = beeStates["coordinator"] == SwarmHiveBeeState.NEEDS,
            beamSeq = beamMsg?.seq,
            beamFrom = beamFrom,
            beamTargets = beamTargets,
            waitLinks = waitLinks
        )
    }
}
