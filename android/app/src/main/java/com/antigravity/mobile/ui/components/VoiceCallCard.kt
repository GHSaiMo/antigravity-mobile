package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.VolumeOff
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.filled.Mic
import androidx.compose.material.icons.filled.MicOff
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.VoiceCallManager
import com.antigravity.mobile.ui.util.VoiceCallState
import com.antigravity.mobile.ui.util.VoiceTranscriptRole
import com.antigravity.mobile.ui.util.rememberHaptic
import java.util.Locale

@Composable
fun VoiceCallCard(
    voiceCall: VoiceCallManager,
    isAgentRunning: Boolean,
    onStopAgentTask: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()

    val callState by voiceCall.callState.collectAsState()
    val durationSeconds by voiceCall.durationSeconds.collectAsState()
    val isMicMuted by voiceCall.isMicMuted.collectAsState()
    val isSpeakerMuted by voiceCall.isSpeakerMuted.collectAsState()
    val playbackSpeed by voiceCall.playbackSpeed.collectAsState()
    val isAgentSpeaking by voiceCall.isAgentSpeaking.collectAsState()
    val soundLevels by voiceCall.soundLevels.collectAsState()
    val liveTranscriptText by voiceCall.liveTranscriptText.collectAsState()
    val liveTranscriptRole by voiceCall.liveTranscriptRole.collectAsState()

    val cyanColor = Color(0xFF06B6D4)
    val greenColor = Color(0xFF10B981)
    val orangeColor = Color(0xFFF59E0B)
    val redColor = Color(0xFFEF4444)

    val statusDotColor = when (callState) {
        VoiceCallState.CONNECTING, VoiceCallState.RECONNECTING -> orangeColor
        VoiceCallState.LISTENING -> if (isAgentSpeaking) cyanColor else greenColor
        VoiceCallState.FAILED -> redColor
        VoiceCallState.IDLE -> colors.textMuted
    }

    val statusTimerLabel = when (callState) {
        VoiceCallState.CONNECTING -> "连接中"
        VoiceCallState.RECONNECTING -> "重连中"
        else -> {
            val minutes = durationSeconds / 60
            val seconds = durationSeconds % 60
            String.format(Locale.US, "%02d:%02d", minutes, seconds)
        }
    }

    val speedLabel = if ((playbackSpeed * 10.0) % 1.0 == 0.0) {
        String.format(Locale.US, "%.1fx", playbackSpeed)
    } else {
        String.format(Locale.US, "%.2fx", playbackSpeed)
    }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(colors.surfaceVariant, RoundedCornerShape(14.dp))
            .border(
                width = 0.75.dp,
                color = if (isAgentSpeaking) cyanColor.copy(alpha = 0.4f) else colors.accentIndigo.copy(alpha = 0.28f),
                shape = RoundedCornerShape(16.dp)
            )
            .padding(horizontal = 14.dp, vertical = 11.dp),
        verticalArrangement = Arrangement.spacedBy(7.dp)
    ) {
        AnimatedVisibility(
            visible = liveTranscriptText.isNotBlank(),
            enter = expandVertically() + fadeIn(),
            exit = shrinkVertically() + fadeOut()
        ) {
            Row(
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 2.dp, bottom = 2.dp)
            ) {
                Box(
                    modifier = Modifier
                        .padding(top = 1.5.dp)
                        .clip(RoundedCornerShape(50))
                        .background(if (liveTranscriptRole == VoiceTranscriptRole.AI) cyanColor else colors.accentIndigo)
                        .padding(horizontal = 6.dp, vertical = 1.5.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = if (liveTranscriptRole == VoiceTranscriptRole.AI) "AI" else "我",
                        color = Color.White,
                        fontSize = 10.sp,
                        fontWeight = FontWeight.Bold
                    )
                }

                Text(
                    text = liveTranscriptText,
                    color = colors.textPrimary.copy(alpha = 0.92f),
                    fontSize = 13.sp,
                    lineHeight = 18.sp,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier
                        .weight(1f)
                        .heightIn(min = 36.dp)
                )
            }
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            // Status dot + MM:SS timer
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                modifier = Modifier.widthIn(min = 58.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(8.dp)
                        .clip(CircleShape)
                        .background(statusDotColor)
                )
                Text(
                    text = statusTimerLabel,
                    color = colors.textPrimary,
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    fontFamily = FontFamily.Monospace
                )
            }

            // Wide dual-color dynamic waveform
            Row(
                modifier = Modifier
                    .weight(1f)
                    .height(26.dp),
                horizontalArrangement = Arrangement.spacedBy(2.5.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                soundLevels.forEach { level ->
                    val barHeight by animateDpAsState(
                        targetValue = (level * 24).coerceIn(4f, 24f).dp,
                        animationSpec = tween(durationMillis = 80),
                        label = "voiceWaveform"
                    )
                    val barColor = when {
                        isMicMuted && !isAgentSpeaking -> colors.textMuted.copy(alpha = 0.35f)
                        isAgentSpeaking -> cyanColor
                        else -> colors.accentIndigo
                    }
                    Box(
                        modifier = Modifier
                            .width(3.dp)
                            .height(barHeight)
                            .clip(RoundedCornerShape(1.5.dp))
                            .background(barColor)
                    )
                }
            }

            // Controls: Speed pill, Mic Mute, Speaker Mute, optional Stop Agent Task
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(7.dp)
            ) {
                Box(
                    modifier = Modifier
                        .height(28.dp)
                        .clip(RoundedCornerShape(50))
                        .background(colors.surface)
                        .clickable {
                            haptic.light()
                            voiceCall.cyclePlaybackSpeed()
                        }
                        .padding(horizontal = 8.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = speedLabel,
                        color = colors.textPrimary,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Bold
                    )
                }

                Box(
                    modifier = Modifier
                        .size(28.dp)
                        .clip(CircleShape)
                        .background(if (isMicMuted) redColor else colors.surface)
                        .clickable {
                            haptic.light()
                            voiceCall.toggleMicMute()
                        },
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = if (isMicMuted) Icons.Default.MicOff else Icons.Default.Mic,
                        contentDescription = if (isMicMuted) "取消静音麦克风" else "静音麦克风",
                        tint = if (isMicMuted) Color.White else colors.textPrimary,
                        modifier = Modifier.size(14.dp)
                    )
                }

                Box(
                    modifier = Modifier
                        .size(28.dp)
                        .clip(CircleShape)
                        .background(if (isSpeakerMuted) orangeColor else colors.surface)
                        .clickable {
                            haptic.light()
                            voiceCall.toggleSpeakerMute()
                        },
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = if (isSpeakerMuted) Icons.AutoMirrored.Filled.VolumeOff else Icons.AutoMirrored.Filled.VolumeUp,
                        contentDescription = if (isSpeakerMuted) "开启AI外放" else "静音AI外放",
                        tint = if (isSpeakerMuted) Color.White else colors.textPrimary,
                        modifier = Modifier.size(14.dp)
                    )
                }

                if (isAgentRunning) {
                    Box(
                        modifier = Modifier
                            .size(28.dp)
                            .clip(CircleShape)
                            .background(redColor.copy(alpha = 0.88f))
                            .clickable {
                                haptic.medium()
                                onStopAgentTask()
                            },
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.Stop,
                            contentDescription = "停止当前任务",
                            tint = Color.White,
                            modifier = Modifier.size(14.dp)
                        )
                    }
                }
            }
        }
    }
}
