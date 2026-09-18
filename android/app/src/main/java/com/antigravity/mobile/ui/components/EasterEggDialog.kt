package com.antigravity.mobile.ui.components

import android.os.SystemClock
import androidx.activity.compose.BackHandler
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.R
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlin.math.cos
import kotlin.math.pow
import kotlin.math.sin
import kotlin.random.Random

// MARK: - Firework Particle & Burst Models

private data class FireworkParticle(
    val angle: Float,
    val speed: Float,
    val color: Color,
    val size: Float,
    val initialAlpha: Float
)

private data class FireworkBurst(
    val offsetX: Float,
    val offsetY: Float,
    val delayMs: Long,
    val durationMs: Long = 1500L,
    val particles: List<FireworkParticle>
) {
    companion object {
        private val colors = listOf(
            Color(0xFFFFD700), // Gold
            Color(0xFFA855F7), // Purple
            Color(0xFF06B6D4), // Cyan
            Color(0xFFF43F5E), // Rose
            Color(0xFF10B981), // Emerald
            Color(0xFFFF9800), // Amber
            Color(0xFFFFFFFF)  // White star
        )

        fun create(offsetX: Float, offsetY: Float, delayMs: Long, count: Int = 30): FireworkBurst {
            val random = Random(System.nanoTime() xor (delayMs * 31))
            val particles = (0 until count).map { i ->
                val angle = (i.toFloat() / count.toFloat()) * 2f * Math.PI.toFloat() + (random.nextFloat() - 0.5f) * 0.3f
                val speed = 80f + random.nextFloat() * 160f
                val color = colors[random.nextInt(colors.size)]
                val size = 4f + random.nextFloat() * 5f
                val initialAlpha = 0.85f + random.nextFloat() * 0.15f
                FireworkParticle(angle, speed, color, size, initialAlpha)
            }
            return FireworkBurst(offsetX, offsetY, delayMs, 1500L, particles)
        }
    }
}

// MARK: - Easter Egg Dialog

@Composable
fun EasterEggDialog(
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val density = LocalDensity.current
    val coroutineScope = rememberCoroutineScope()

    val alphaAnim = remember { Animatable(0f) }
    val scaleAnim = remember { Animatable(0.82f) }
    var isDismissing by remember { mutableStateOf(false) }

    val startTimestamp = remember { SystemClock.elapsedRealtime() }
    val currentTime = produceState(initialValue = startTimestamp) {
        while (isActive) {
            withFrameNanos {
                value = SystemClock.elapsedRealtime()
            }
        }
    }

    val bursts = remember {
        listOf(
            FireworkBurst.create(-80f, -90f, 0L, 30),
            FireworkBurst.create(85f, -75f, 150L, 32),
            FireworkBurst.create(-75f, 70f, 350L, 28),
            FireworkBurst.create(80f, 85f, 500L, 30),
            FireworkBurst.create(0f, -120f, 750L, 36)
        )
    }

    val dismissWithAnimation: () -> Unit = {
        if (!isDismissing) {
            isDismissing = true
            coroutineScope.launch {
                launch {
                    alphaAnim.animateTo(0f, animationSpec = tween(550, easing = FastOutSlowInEasing))
                }
                launch {
                    scaleAnim.animateTo(0.92f, animationSpec = tween(550, easing = FastOutSlowInEasing))
                }
                delay(600)
                onDismiss()
            }
        }
    }

    LaunchedEffect(Unit) {
        // Entrance animation
        launch {
            alphaAnim.animateTo(1f, animationSpec = tween(350, easing = FastOutSlowInEasing))
        }
        launch {
            scaleAnim.animateTo(1f, animationSpec = spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessMedium))
        }
        // Auto fade-out after ~2.3 seconds
        delay(2300)
        dismissWithAnimation()
    }

    Dialog(
        onDismissRequest = { /* Absorb clicks so rapid taps don't close early */ },
        properties = DialogProperties(
            usePlatformDefaultWidth = false,
            decorFitsSystemWindows = false
        )
    ) {
        BackHandler {
            dismissWithAnimation()
        }

        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(Color.Black.copy(alpha = 0.45f * alphaAnim.value))
                .clickable(
                    interactionSource = remember { MutableInteractionSource() },
                    indication = null,
                    onClick = { /* Absorb clicks so taps exceeding 10 do not dismiss early */ }
                ),
            contentAlignment = Alignment.Center
        ) {
            // Fireworks Canvas Layer
            Canvas(
                modifier = Modifier
                    .fillMaxSize()
                    .alpha(alphaAnim.value)
            ) {
                val cx = size.width / 2f
                val cy = size.height / 2f
                val elapsed = currentTime.value - startTimestamp

                bursts.forEach { burst ->
                    val burstElapsed = elapsed - burst.delayMs
                    if (burstElapsed in 1..burst.durationMs) {
                        val progress = burstElapsed.toFloat() / burst.durationMs.toFloat()
                        // Deceleration curve
                        val ease = 1f - (1f - progress).pow(3)
                        // Slight gravity
                        val gravity = progress.pow(2) * density.run { 35.dp.toPx() }

                        val burstOriginX = cx + density.run { burst.offsetX.dp.toPx() }
                        val burstOriginY = cy + density.run { burst.offsetY.dp.toPx() }

                        burst.particles.forEach { particle ->
                            val distance = density.run { particle.speed.dp.toPx() } * ease
                            val px = burstOriginX + cos(particle.angle) * distance
                            val py = burstOriginY + sin(particle.angle) * distance + gravity

                            val currentAlpha = particle.initialAlpha * (1f - progress).coerceAtLeast(0f)
                            val currentRadius = density.run { (particle.size * (1f - progress * 0.6f).coerceAtLeast(0.3f)).dp.toPx() } / 2f

                            drawCircle(
                                color = particle.color.copy(alpha = currentAlpha),
                                radius = currentRadius,
                                center = Offset(px, py)
                            )

                            // Inner bright core
                            if (particle.size > 6f && progress < 0.6f) {
                                drawCircle(
                                    color = Color.White.copy(alpha = currentAlpha * 0.8f),
                                    radius = currentRadius * 0.5f,
                                    center = Offset(px, py)
                                )
                            }
                        }
                    }
                }
            }

            // Centered Easter Egg Card
            Surface(
                shape = RoundedCornerShape(24.dp),
                color = colors.surface.copy(alpha = 0.94f),
                border = BorderStroke(1.dp, Color.White.copy(alpha = 0.18f)),
                shadowElevation = 24.dp,
                modifier = Modifier
                    .padding(32.dp)
                    .scale(scaleAnim.value)
                    .alpha(alphaAnim.value)
            ) {
                Column(
                    modifier = Modifier.padding(horizontal = 36.dp, vertical = 28.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(16.dp)
                ) {
                    // App Logo
                    Image(
                        painter = painterResource(id = R.drawable.app_logo),
                        contentDescription = "Multigravity Logo",
                        modifier = Modifier
                            .size(72.dp)
                            .clip(RoundedCornerShape(18.dp))
                            .border(1.dp, Color.White.copy(alpha = 0.2f), RoundedCornerShape(18.dp))
                    )

                    // Title
                    Text(
                        text = "Multigravity",
                        fontSize = 22.sp,
                        fontWeight = FontWeight.Bold,
                        color = colors.textPrimary,
                        letterSpacing = 0.5.sp
                    )

                    // Subtitle / Credit
                    Surface(
                        shape = CircleShape,
                        color = colors.accentIndigo.copy(alpha = 0.12f)
                    ) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp),
                            modifier = Modifier.padding(horizontal = 14.dp, vertical = 6.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.AutoAwesome,
                                contentDescription = null,
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(14.dp)
                            )
                            Text(
                                text = "Design by Jiuge",
                                fontSize = 14.sp,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.accentIndigo
                            )
                        }
                    }
                }
            }
        }
    }
}
