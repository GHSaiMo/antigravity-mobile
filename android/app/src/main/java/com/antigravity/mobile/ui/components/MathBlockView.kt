package com.antigravity.mobile.ui.components

import android.widget.Toast
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.DisableSelection
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.service.MathSymbolProcessor
import com.antigravity.mobile.ui.theme.AppColors
import com.antigravity.mobile.ui.util.loc
import com.antigravity.mobile.ui.util.rememberHaptic

/**
 * Native display math block (`$$ ... $$` / `\[ ... \]`), 1:1 aligned with iOS MathBlockView.
 * Renders cached high-DPI KaTeX bitmaps via KaTeXRenderEngine with instant Unicode fallback
 * while rendering or on incomplete streaming syntax, plus horizontal scrolling for wide formulas.
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun MathBlockView(
    latex: String,
    colors: AppColors,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    val density = LocalDensity.current.density
    val clipboardManager = LocalClipboardManager.current
    val haptic = rememberHaptic()
    val isExport = LocalShareExport.current
    val bodyScale = if (isExport) 1.0f else LocalMarkdownBodyScale.current
    val fontSizeSp = 16.5f * bodyScale

    var rendered by remember(latex, fontSizeSp, colors.isDark) {
        mutableStateOf(
            KaTeXRenderEngine.cached(
                latex = latex,
                displayMode = true,
                fontSizeSp = fontSizeSp,
                isDark = colors.isDark
            )
        )
    }

    LaunchedEffect(latex, fontSizeSp, colors.isDark, isExport) {
        if (isExport || rendered != null) return@LaunchedEffect
        rendered = KaTeXRenderEngine.render(
            context = context,
            latex = latex,
            displayMode = true,
            fontSizeSp = fontSizeSp,
            density = density,
            isDark = colors.isDark
        )
    }

    val shape = RoundedCornerShape(10.dp)
    val cardBg = colors.codeBlockBg.copy(alpha = if (colors.isDark) 0.72f else 0.85f)
    val cardBorder = colors.border.copy(alpha = 0.65f)

    DisableSelection {
        BoxWithConstraints(
            modifier = modifier
                .fillMaxWidth()
                .clip(shape)
                .background(cardBg)
                .border(0.8.dp, cardBorder, shape)
                .then(
                    if (isExport) Modifier else Modifier.combinedClickable(
                        onClick = {},
                        onLongClick = {
                            haptic.medium()
                            clipboardManager.setText(AnnotatedString(latex.trim()))
                            Toast.makeText(context, "已复制 LaTeX 公式".loc, Toast.LENGTH_SHORT).show()
                        }
                    )
                )
                .padding(vertical = 12.dp),
            contentAlignment = Alignment.Center
        ) {
            val availWidthDp = (maxWidth - 28.dp).coerceAtLeast(1.dp)
            val formula = rendered
            if (formula != null) {
                val colorFilter = if (formula.isMonochrome) ColorFilter.tint(colors.textPrimary) else null
                val needsScroll = !isExport && formula.widthDp.dp > availWidthDp
                if (isExport) {
                    val scale = if (formula.widthDp.dp > availWidthDp && formula.widthDp > 0f) {
                        availWidthDp.value / formula.widthDp
                    } else {
                        1f
                    }
                    Image(
                        bitmap = formula.imageBitmap,
                        contentDescription = null,
                        contentScale = ContentScale.Fit,
                        colorFilter = colorFilter,
                        modifier = Modifier
                            .padding(horizontal = 14.dp)
                            .size(
                                width = (formula.widthDp * scale).dp,
                                height = (formula.heightDp * scale).dp
                            )
                    )
                } else if (needsScroll) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .horizontalScroll(rememberScrollState())
                            .padding(horizontal = 14.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Image(
                            bitmap = formula.imageBitmap,
                            contentDescription = null,
                            contentScale = ContentScale.Fit,
                            colorFilter = colorFilter,
                            modifier = Modifier.size(
                                width = formula.widthDp.dp,
                                height = formula.heightDp.dp
                            )
                        )
                    }
                } else {
                    Image(
                        bitmap = formula.imageBitmap,
                        contentDescription = null,
                        contentScale = ContentScale.Fit,
                        colorFilter = colorFilter,
                        modifier = Modifier
                            .padding(horizontal = 14.dp)
                            .size(
                                width = formula.widthDp.dp,
                                height = formula.heightDp.dp
                            )
                    )
                }
            } else {
                val fallbackText = remember(latex) {
                    MathSymbolProcessor.cleanMathExpression(latex.replace("\\\\", "\n"))
                }
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .then(if (isExport) Modifier else Modifier.horizontalScroll(rememberScrollState()))
                        .padding(horizontal = 14.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = fallbackText,
                        color = colors.textPrimary,
                        fontSize = (15.5f * bodyScale).sp,
                        fontFamily = FontFamily.Serif,
                        lineHeight = (22f * bodyScale).sp,
                        textAlign = TextAlign.Center
                    )
                }
            }
        }
    }
}
