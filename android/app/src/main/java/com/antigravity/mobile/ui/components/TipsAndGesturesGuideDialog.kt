package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.loc

@Composable
fun TipsAndGesturesSubpageContent() {
    val colors = AntigravityTheme.colors
    fun badgeColor(light: Color, dark: Color): Color = if (colors.isDark) dark else light

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 14.dp)
            .navigationBarsPadding(),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        // Hero
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = 6.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Box(
                modifier = Modifier
                    .size(56.dp)
                    .clip(CircleShape)
                    .background(Color(0xFFF59E0B).copy(alpha = 0.15f)),
                contentAlignment = Alignment.Center
            ) {
                Icon(
                    imageVector = Icons.Default.Lightbulb,
                    contentDescription = null,
                    tint = Color(0xFFF59E0B),
                    modifier = Modifier.size(28.dp)
                )
            }
            Text(
                text = "使用技巧与快捷手势".loc,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textPrimary
            )
            Text(
                text = "掌握核心隐蔽手势与快捷操作，无需繁琐步骤，在日常交互中获得更高效的陪伴体验。".loc,
                fontSize = 13.sp,
                color = colors.textMuted,
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(horizontal = 8.dp)
            )
        }

        // Section 1
        GuideCategory(title = "会话列表管理".loc) {
            GuideRow(
                icon = Icons.Default.Delete,
                badgeColor = badgeColor(
                    light = Color(0xFFEF4444), // 浅色模式恢复正常饱和度红
                    dark = Color(0xFFA85C5C) // 深色模式保持灰调砖红
                ),
                title = "左滑会话卡片".loc,
                desc = "向左轻扫任意会话，呼出快捷删除操作。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.PushPin,
                badgeColor = badgeColor(
                    light = Color(0xFFF97316), // 浅色模式恢复正常饱和度橙
                    dark = Color(0xFFAE7A52) // 深色模式保持灰调暖橙
                ),
                title = "右滑固定置顶".loc,
                desc = "向右轻扫会话卡片，一键将当前重点会话固定在列表最顶端。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.Folder,
                badgeColor = badgeColor(
                    light = Color(0xFF10B981), // 浅色模式恢复清透翡翠绿
                    dark = Color(0xFF598070) // 深色模式保持沉稳鼠尾草绿
                ),
                title = "点击左下角项目图标".loc,
                desc = "精准轻按卡片左下角的文件夹或气泡图标，一键聚合筛选该项目的所有会话；在聚合模式下，点击「新建对话」即可直接在该项目里新建对话。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.Archive,
                badgeColor = badgeColor(
                    light = Color(0xFF64748B), // 浅色模式与左滑归档按钮一致的石板灰蓝
                    dark = Color(0xFF5A6678) // 深色模式保持沉稳低饱和石板灰蓝
                ),
                title = "左滑归档会话".loc,
                desc = "向左轻扫会话卡片可一键归档，保持列表清爽；在聚合模式下选择「已归档」，即可随时找到已归档的会话。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.Computer,
                badgeColor = badgeColor(
                    light = Color(0xFF6366F1),
                    dark = Color(0xFF61709E)
                ),
                title = "多主机快捷切换".loc,
                desc = "长按顶部「Multigravity」标题并下滑可添加或切换多台电脑网关；绑定 2 台及以上主机后，左右轻扫标题可快速轮换，列表上滑收拢后轻点居中主机名也可一键展开切换面板。".loc
            )
        }

        // Section 2
        GuideCategory(title = "对话与模型调度".loc) {
            GuideRow(
                icon = Icons.Default.Sync,
                badgeColor = badgeColor(
                    light = Color(0xFF3B82F6), // 浅色模式恢复正常饱和度蓝
                    dark = Color(0xFF527BA3) // 深色模式保持灰调柔蓝
                ),
                title = "短按模型胶囊".loc,
                desc = "轻按输入框底部的模型胶囊，在已设定的首选与次选模型（如 Gemini / Claude）之间一键极速轮换。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.AutoAwesome,
                badgeColor = badgeColor(
                    light = Color(0xFF6366F1), // 浅色模式靛蓝与建议胶囊一致
                    dark = Color(0xFF6B70B8) // 深色模式保持灰调靛蓝
                ),
                title = "提示词建议胶囊".loc,
                desc = "短按建议胶囊可将其填入输入框方便修改；长按 1.5 秒待进度条走满后直接发送，提前松手即可取消防误触。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.ArrowUpward,
                badgeColor = badgeColor(
                    light = Color(0xFFDE7342), // 浅色模式 Claude 品牌暖橙
                    dark = Color(0xFFA86E54) // 深色模式低饱和 Claude 陶土橙
                ),
                title = "长按模型胶囊上滑".loc,
                desc = "按住模型胶囊 0.2 秒后向上滑动，即可呼出全量模型转盘与思考强度选单，松手即刻选定。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.PostAdd,
                badgeColor = badgeColor(
                    light = Color(0xFF14B8A6), // 浅色模式恢复正常饱和度青
                    dark = Color(0xFF477D85) // 深色模式保持灰青水鸭
                ),
                title = "Workflow 提示词模板".loc,
                desc = "短按打开满屏提示词模板库；长按上滑可极速将高频工程模板填入输入框。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.Build,
                badgeColor = badgeColor(
                    light = Color(0xFF6366F1), // 浅色模式恢复正常饱和度靛蓝
                    dark = Color(0xFF597080) // 深色模式保持蓝灰深青
                ),
                title = "会话右上角工具与任务看板".loc,
                desc = "轻点秒开控制看板，随时查看当前 MCP 工具、系统技能与后台任务；长按可快捷切换全局权限模式（严格审批 / 沙箱 / 全自动）或开启旁路快问。".loc
            )
        }

        // Section 3
        GuideCategory(title = "照片与媒体功能".loc) {
            GuideRow(
                icon = Icons.Default.PhotoLibrary,
                badgeColor = badgeColor(
                    light = Color(0xFF22C55E), // 浅色模式恢复正常饱和度绿
                    dark = Color(0xFF618061) // 深色模式保持灰橄榄绿
                ),
                title = "相册顶部标题切换文件夹".loc,
                desc = "在照片选择抽屉中，轻点顶部「最近项目」标题即可呼出相册选择器，精准定位「截屏」或指定相册，无需无休止向上滑动。".loc
            )
            GuideDivider()
            GuideRow(
                icon = Icons.Default.Mic,
                badgeColor = badgeColor(
                    light = Color(0xFFEC4899), // 浅色模式恢复正常饱和度玫红
                    dark = Color(0xFFA6616B) // 深色模式保持灰调干枯玫瑰
                ),
                title = "双模语音交互".loc,
                desc = "空闲时轻按右下角麦克风开启实时全双工语音通话；长按则直接唤起系统高精度语音听写转文字。".loc
            )
        }

        Spacer(modifier = Modifier.height(16.dp))
    }
}

@Composable
private fun GuideCategory(
    title: String,
    content: @Composable ColumnScope.() -> Unit
) {
    val colors = AntigravityTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(
            text = title.uppercase(),
            fontSize = 12.sp,
            fontWeight = FontWeight.SemiBold,
            color = colors.textSecondary,
            modifier = Modifier.padding(horizontal = 4.dp)
        )
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(12.dp))
                .background(colors.surface)
                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp)),
            content = content
        )
    }
}

@Composable
private fun GuideDivider() {
    val colors = AntigravityTheme.colors
    HorizontalDivider(
        color = colors.separator.copy(alpha = 0.35f),
        thickness = 0.5.dp,
        modifier = Modifier.padding(start = 60.dp)
    )
}

@Composable
private fun GuideRow(
    icon: ImageVector,
    badgeColor: Color,
    title: String,
    desc: String
) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        verticalAlignment = Alignment.Top
    ) {
        Box(
            modifier = Modifier
                .padding(top = 2.dp)
                .size(30.dp)
                .clip(RoundedCornerShape(7.dp))
                .background(badgeColor),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = Color.White,
                modifier = Modifier.size(17.dp)
            )
        }
        Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(
                text = title,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textPrimary
            )
            Text(
                text = desc,
                fontSize = 13.sp,
                color = colors.textSecondary,
                lineHeight = 18.sp
            )
        }
    }
}
