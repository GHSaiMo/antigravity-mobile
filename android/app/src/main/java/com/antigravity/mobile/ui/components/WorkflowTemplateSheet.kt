package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.filled.BugReport
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Security
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic

data class WorkflowTemplateEntry(
    val id: String,
    val title: String,
    val subtitle: String,
    val icon: ImageVector,
    val iconColor: Color,
    val templatePrompt: String
)

/**
 * 动态多智能体工作流预设模板选择抽屉 (WorkflowTemplateSheet)。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WorkflowTemplateSheet(
    onDismissRequest: () -> Unit,
    onSelectTemplate: (String) -> Unit
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = false)
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()

    val templates = listOf(
        WorkflowTemplateEntry(
            id = "code_review",
            title = "并发代码审查",
            subtitle = "并行启动安全、性能、类型规范多角度 Sub-Agents 进行深度审计",
            icon = Icons.Default.Security,
            iconColor = colors.accentIndigo,
            templatePrompt = "/workflow 请使用 run_workflow 动态编排工作流，以 parallel 并发对 [模块/文件路径] 进行多维度审计（安全/性能/类型规范），并在最终阶段输出结构化修复建议。"
        ),
        WorkflowTemplateEntry(
            id = "test_and_fix",
            title = "并发单测与故障修复",
            subtitle = "并行运行各模块单元测试，定位失败用例并尝试由子代理生成补丁",
            icon = Icons.Default.BugReport,
            iconColor = colors.accentGreen,
            templatePrompt = "/workflow 请使用 run_workflow 编排自动化单测，以 parallel 同时运行 [测试目标或路径] 的单元测试，聚合失败信息并生成修复方案。"
        ),
        WorkflowTemplateEntry(
            id = "research",
            title = "多维度综合调研",
            subtitle = "分派多个子代理从技术实现、竞品方案、生产实践分头研究并归纳",
            icon = Icons.Default.Search,
            iconColor = colors.accentBlue,
            templatePrompt = "/workflow 请使用 run_workflow 编排调研工作流，分派子代理分别调查 [调研主题] 的核心技术方案、竞品对比与最佳实践，并汇总为结构化报告。"
        ),
        WorkflowTemplateEntry(
            id = "custom",
            title = "自定义动态工作流",
            subtitle = "自由描述任务，让 Agent 依据依赖自主规划 parallel / pipeline 阶段",
            icon = Icons.Default.AccountTree,
            iconColor = Color(0xFF9C27B0),
            templatePrompt = "/workflow 请使用 run_workflow 编写 Python 动态工作流脚本，分阶段执行以下任务：[描述具体任务]，要求使用 parallel 并发加速并给出 finish 契约结果。"
        )
    )

    ModalBottomSheet(
        onDismissRequest = onDismissRequest,
        sheetState = sheetState,
        containerColor = colors.surface
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 32.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            // Header
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(36.dp)
                        .clip(CircleShape)
                        .background(colors.accentIndigo.copy(alpha = 0.18f)),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.AccountTree,
                        contentDescription = null,
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(20.dp)
                    )
                }

                Column {
                    Text(
                        text = "Antigravity 动态工作流",
                        style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold),
                        color = colors.textPrimary
                    )
                    Text(
                        text = "选择常用模板快速分派多智能体并发任务",
                        style = MaterialTheme.typography.bodySmall,
                        color = colors.textSecondary
                    )
                }
            }

            Spacer(modifier = Modifier.height(4.dp))

            // Template Entries
            templates.forEach { item ->
                Surface(
                    shape = RoundedCornerShape(12.dp),
                    color = colors.surfaceVariant.copy(alpha = 0.6f),
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable {
                            haptic.medium()
                            onSelectTemplate(item.templatePrompt)
                            onDismissRequest()
                        }
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        Box(
                            modifier = Modifier
                                .size(40.dp)
                                .clip(RoundedCornerShape(10.dp))
                                .background(item.iconColor.copy(alpha = 0.15f)),
                            contentAlignment = Alignment.Center
                        ) {
                            Icon(
                                imageVector = item.icon,
                                contentDescription = null,
                                tint = item.iconColor,
                                modifier = Modifier.size(20.dp)
                            )
                        }

                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = item.title,
                                style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
                                color = colors.textPrimary
                            )
                            Spacer(modifier = Modifier.height(2.dp))
                            Text(
                                text = item.subtitle,
                                style = MaterialTheme.typography.bodySmall,
                                color = colors.textSecondary
                            )
                        }

                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }
            }
        }
    }
}
