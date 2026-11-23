package com.antigravity.mobile.ui.components

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.offset
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.pulltorefresh.PullToRefreshDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first

private const val INDICATOR_HIDE_THRESHOLD = 0.02f
private val INDICATOR_SHADOW_CLEARANCE = 10.dp

/**
 * Clean wrapper around [PullToRefreshDefaults.Indicator] that prevents
 * Material3 1.3.1 / Compose 1.7.0 from leaking a lingering shadow tail below
 * the top boundary after the refresh spinner retracts:
 *
 * 1. Tucks the indicator slightly higher behind a top-only clip boundary so
 *    its downward elevation shadow is already fully clipped before crossing
 *    the unmount threshold.
 * 2. Completely removes [PullToRefreshDefaults.Indicator] (and its RenderNode)
 *    from composition whenever idle/retracted (`distanceFraction <= 0.02f`),
 *    eliminating ghost shadow elevation on Android hardware layers.
 * 3. Cooperatively ensures `state.distanceFraction` finishes at exact `0f`
 *    after refresh ends or early-returns without interrupting Material3's
 *    internal `PullToRefreshModifierNode.animateToHidden()` cleanup.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MgyPullToRefreshIndicator(
    state: PullToRefreshState,
    isRefreshing: Boolean,
    modifier: Modifier = Modifier,
    refreshTriggerToken: Int = 0,
    containerColor: Color = PullToRefreshDefaults.containerColor,
    color: Color = PullToRefreshDefaults.indicatorColor,
    threshold: Dp = PullToRefreshDefaults.PositionalThreshold,
) {
    val currentIsRefreshing by rememberUpdatedState(isRefreshing)
    var wasRefreshing by remember { mutableStateOf(isRefreshing) }

    LaunchedEffect(isRefreshing) {
        if (!isRefreshing) {
            if (wasRefreshing) {
                // Let PullToRefreshModifierNode.update() start and finish its own
                // animateToHidden() so it resets internal distancePulled/verticalOffset.
                withFrameNanos { }
                withFrameNanos { }
                if (state.isAnimating) {
                    snapshotFlow { state.isAnimating }.first { !it }
                }
                if (!currentIsRefreshing && state.distanceFraction > 0f && !state.isAnimating) {
                    state.animateToHidden()
                    if (state.distanceFraction != 0f) {
                        state.snapTo(0f)
                    }
                }
            } else if (state.distanceFraction > 0f && !state.isAnimating) {
                // Clean up stale saved distanceFraction after backstack restore.
                state.snapTo(0f)
            }
        }
        wasRefreshing = isRefreshing
    }

    LaunchedEffect(refreshTriggerToken) {
        if (refreshTriggerToken > 0) {
            // Guard against fast-completing or early-returning onRefresh (e.g. local draft)
            // where isRefreshing never stays true across a frame (AOSP b/359949836).
            delay(120L)
            if (!currentIsRefreshing && !state.isAnimating && state.distanceFraction > 0f) {
                state.animateToHidden()
                if (state.distanceFraction != 0f) {
                    state.snapTo(0f)
                }
            }
        }
    }

    val isVisible by remember(state) {
        derivedStateOf { state.distanceFraction > INDICATOR_HIDE_THRESHOLD }
    }

    if (isVisible) {
        Box(
            modifier = modifier.drawWithContent {
                clipRect(
                    top = 0f,
                    left = -Float.MAX_VALUE,
                    right = Float.MAX_VALUE,
                    bottom = Float.MAX_VALUE
                ) {
                    this@drawWithContent.drawContent()
                }
            }
        ) {
            PullToRefreshDefaults.Indicator(
                state = state,
                isRefreshing = isRefreshing,
                modifier = Modifier.offset(y = -INDICATOR_SHADOW_CLEARANCE),
                containerColor = containerColor,
                color = color,
                threshold = threshold,
            )
        }
    }
}
