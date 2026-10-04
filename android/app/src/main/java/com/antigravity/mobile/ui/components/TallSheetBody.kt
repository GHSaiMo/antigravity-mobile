package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

/**
 * Body for the tall, handle-style bottom sheets (settings, quota, new conversation, document and
 * markdown preview).
 *
 * The enclosing [androidx.compose.material3.ModalBottomSheet] must be full height with a
 * transparent container (see [TallSheetDefaults]); this body then draws the visible card flush with
 * the bottom edge and leaves a gap on top that clears the status bar / display cutout, in both
 * portrait and landscape. Sizing the sheet with a fraction of the screen height instead leaves the
 * Material3 sheet pinned to the top, which hid the handle behind the camera and left a strip at the
 * bottom.
 */
@Composable
fun TallSheetBody(
    containerColor: Color,
    onDismiss: () -> Unit,
    content: @Composable ColumnScope.() -> Unit
) {
    val topInset = WindowInsets.safeDrawing.asPaddingValues().calculateTopPadding()
    val topGap = topInset + 8.dp
    Box(modifier = Modifier.fillMaxSize()) {
        // Transparent strip above the card: tapping it dismisses, like tapping the scrim.
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(topGap)
                .clickable(
                    interactionSource = remember { MutableInteractionSource() },
                    indication = null,
                    onClick = onDismiss
                )
        )
        Column(
            modifier = Modifier
                .align(Alignment.BottomCenter)
                .fillMaxSize()
                .padding(top = topGap)
                .clip(RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp))
                .background(containerColor),
            content = content
        )
    }
}

/** Handle indicator for tall sheets that used ModalBottomSheet's dragHandle slot. */
@Composable
fun SheetGrabHandle(color: Color) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 10.dp, bottom = 6.dp),
        contentAlignment = Alignment.Center
    ) {
        Box(
            modifier = Modifier
                .width(36.dp)
                .height(5.dp)
                .clip(CircleShape)
                .background(color)
        )
    }
}
