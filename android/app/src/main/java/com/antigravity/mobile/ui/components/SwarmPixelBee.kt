package com.antigravity.mobile.ui.components

import android.graphics.Bitmap
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlinx.coroutines.delay
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
