package com.antigravity.mobile.data.service

import androidx.compose.ui.graphics.Color

data class FileTypeInfo(
    val displayName: String,
    val color: Color,
    val glyph: String = ""
)

/**
 * 1:1 Kotlin port of iOS FileIconResolver.swift with VS Code dark theme styling.
 * Maps 180+ programming language tags, file extensions, and special filenames
 * to stylized display names and distinct VS Code brand accent colors.
 */
object FileIconResolver {

    private val defaultInfo = FileTypeInfo(
        displayName = "CODE",
        color = Color(0xFF9DA5B4)
    )

    fun resolve(tagOrPath: String): FileTypeInfo {
        var str = tagOrPath.trim().trim('`', '\'', '"')
        if (str.isEmpty()) return defaultInfo

        // Strip URI prefixes / suffixes
        if (str.startsWith("file://")) str = str.removePrefix("file://")
        if (str.contains('#')) str = str.substringBefore('#')
        if (str.contains('?')) str = str.substringBefore('?')
        str = str.replace(Regex(""":(?:L?\d+(?:-[a-zA-Z]?\d+)?|\d+(?::\d+)?)$"""), "")
        str = str.trim()

        val lower = str.lowercase()
        val filename = if (lower.contains('/')) lower.substringAfterLast('/') else lower

        // 1. Check exact filename matches
        specialFileNames[filename]?.let { return it }

        // 2. Check extension or language tag
        val ext = if (filename.contains('.')) filename.substringAfterLast('.') else filename
        extensionMap[ext]?.let { return it }

        return FileTypeInfo(
            displayName = str.uppercase(),
            color = Color(0xFF9DA5B4)
        )
    }

    private val specialFileNames = mapOf(
        "dockerfile" to FileTypeInfo("Dockerfile", Color(0xFF2496ED), "🐳"),
        "docker-compose.yml" to FileTypeInfo("Docker Compose", Color(0xFF2496ED), "🐳"),
        "docker-compose.yaml" to FileTypeInfo("Docker Compose", Color(0xFF2496ED), "🐳"),
        "makefile" to FileTypeInfo("Makefile", Color(0xFF427819), "⚙️"),
        "cmakelists.txt" to FileTypeInfo("CMake", Color(0xFF064F8C), "⚙️"),
        "package.json" to FileTypeInfo("npm Package", Color(0xFFCB3837), "📦"),
        "package-lock.json" to FileTypeInfo("npm Lock", Color(0xFFCB3837), "🔒"),
        "pnpm-lock.yaml" to FileTypeInfo("pnpm Lock", Color(0xFFF69220), "🔒"),
        "yarn.lock" to FileTypeInfo("Yarn Lock", Color(0xFF2C8EBB), "🔒"),
        "cargo.toml" to FileTypeInfo("Cargo Manifest", Color(0xFFDEA584), "🦀"),
        "cargo.lock" to FileTypeInfo("Cargo Lock", Color(0xFFDEA584), "🔒"),
        "gemfile" to FileTypeInfo("Gemfile", Color(0xFFCC342D), "💎"),
        "podfile" to FileTypeInfo("CocoaPods", Color(0xFFEE3322), "📦"),
        "podfile.lock" to FileTypeInfo("Pods Lock", Color(0xFFEE3322), "🔒"),
        "build.gradle" to FileTypeInfo("Gradle Build", Color(0xFF02303A), "🐘"),
        "build.gradle.kts" to FileTypeInfo("Gradle Kotlin", Color(0xFF7F52FF), "🐘"),
        "settings.gradle" to FileTypeInfo("Gradle Settings", Color(0xFF02303A), "🐘"),
        "settings.gradle.kts" to FileTypeInfo("Gradle Settings", Color(0xFF7F52FF), "🐘"),
        ".gitignore" to FileTypeInfo("Git Ignore", Color(0xFFF05032), "🙈"),
        ".gitattributes" to FileTypeInfo("Git Attributes", Color(0xFFF05032), "🏷️"),
        ".env" to FileTypeInfo("Environment", Color(0xFFECD53F), "🔑"),
        ".eslintrc" to FileTypeInfo("ESLint", Color(0xFF4B32C3), "⚡"),
        ".prettierrc" to FileTypeInfo("Prettier", Color(0xFF56B3B4), "✨"),
        "tsconfig.json" to FileTypeInfo("TS Config", Color(0xFF3178C6), "⚙️"),
        "implementation_plan.md" to FileTypeInfo("Plan", Color(0xFF6366F1), "📋"),
        "walkthrough.md" to FileTypeInfo("Walkthrough", Color(0xFF10B981), "🚶")
    )

    private val extensionMap = mapOf(
        // Kotlin
        "kt" to FileTypeInfo("Kotlin", Color(0xFF7F52FF)),
        "kts" to FileTypeInfo("Kotlin Script", Color(0xFF7F52FF)),
        "kotlin" to FileTypeInfo("Kotlin", Color(0xFF7F52FF)),

        // Swift
        "swift" to FileTypeInfo("Swift", Color(0xFFF05138)),

        // Python
        "py" to FileTypeInfo("Python", Color(0xFF3776AB)),
        "pyw" to FileTypeInfo("Python", Color(0xFF3776AB)),
        "python" to FileTypeInfo("Python", Color(0xFF3776AB)),
        "ipynb" to FileTypeInfo("Jupyter", Color(0xFFDA5B0B)),

        // TypeScript & React
        "ts" to FileTypeInfo("TypeScript", Color(0xFF3178C6)),
        "mts" to FileTypeInfo("TypeScript Module", Color(0xFF3178C6)),
        "cts" to FileTypeInfo("TypeScript CommonJS", Color(0xFF3178C6)),
        "typescript" to FileTypeInfo("TypeScript", Color(0xFF3178C6)),
        "tsx" to FileTypeInfo("React TSX", Color(0xFF007ACC)),

        // JavaScript & React
        "js" to FileTypeInfo("JavaScript", Color(0xFFF7DF1E)),
        "mjs" to FileTypeInfo("JavaScript Module", Color(0xFFF7DF1E)),
        "cjs" to FileTypeInfo("JavaScript CommonJS", Color(0xFFF7DF1E)),
        "javascript" to FileTypeInfo("JavaScript", Color(0xFFF7DF1E)),
        "jsx" to FileTypeInfo("React JSX", Color(0xFF61DAFB)),

        // Go
        "go" to FileTypeInfo("Go", Color(0xFF00ADD8)),
        "golang" to FileTypeInfo("Go", Color(0xFF00ADD8)),

        // Rust
        "rs" to FileTypeInfo("Rust", Color(0xFFDEA584)),
        "rust" to FileTypeInfo("Rust", Color(0xFFDEA584)),

        // Java
        "java" to FileTypeInfo("Java", Color(0xFFE76F00)),
        "jar" to FileTypeInfo("Java Archive", Color(0xFFE76F00)),

        // C / C++
        "c" to FileTypeInfo("C", Color(0xFF555555)),
        "h" to FileTypeInfo("C Header", Color(0xFF555555)),
        "cpp" to FileTypeInfo("C++", Color(0xFF00599C)),
        "cc" to FileTypeInfo("C++", Color(0xFF00599C)),
        "cxx" to FileTypeInfo("C++", Color(0xFF00599C)),
        "hpp" to FileTypeInfo("C++ Header", Color(0xFF00599C)),
        "hxx" to FileTypeInfo("C++ Header", Color(0xFF00599C)),

        // C# & .NET
        "cs" to FileTypeInfo("C#", Color(0xFF178600)),
        "csharp" to FileTypeInfo("C#", Color(0xFF178600)),
        "fs" to FileTypeInfo("F#", Color(0xFFB845FC)),
        "vb" to FileTypeInfo("Visual Basic", Color(0xFF945DB7)),

        // Dart & Flutter
        "dart" to FileTypeInfo("Dart", Color(0xFF0175C2)),

        // Ruby
        "rb" to FileTypeInfo("Ruby", Color(0xFFCC342D)),
        "ruby" to FileTypeInfo("Ruby", Color(0xFFCC342D)),

        // PHP
        "php" to FileTypeInfo("PHP", Color(0xFF4F5D95)),

        // Web Frameworks
        "vue" to FileTypeInfo("Vue", Color(0xFF41B883)),
        "svelte" to FileTypeInfo("Svelte", Color(0xFFFF3E00)),
        "astro" to FileTypeInfo("Astro", Color(0xFFFF5D01)),

        // Web Markup & Styling
        "html" to FileTypeInfo("HTML", Color(0xFFE34F26)),
        "htm" to FileTypeInfo("HTML", Color(0xFFE34F26)),
        "css" to FileTypeInfo("CSS", Color(0xFF1572B6)),
        "scss" to FileTypeInfo("SCSS", Color(0xFFCD6799)),
        "sass" to FileTypeInfo("Sass", Color(0xFFCD6799)),
        "less" to FileTypeInfo("Less", Color(0xFF1D365D)),
        "styl" to FileTypeInfo("Stylus", Color(0xFFff6347)),

        // Shell Scripting
        "sh" to FileTypeInfo("Shell", Color(0xFF4EAA25)),
        "bash" to FileTypeInfo("Bash", Color(0xFF4EAA25)),
        "zsh" to FileTypeInfo("Zsh", Color(0xFF4EAA25)),
        "shell" to FileTypeInfo("Shell", Color(0xFF4EAA25)),
        "fish" to FileTypeInfo("Fish", Color(0xFF4EAA25)),
        "ps1" to FileTypeInfo("PowerShell", Color(0xFF012456)),
        "bat" to FileTypeInfo("Batch", Color(0xFFC1F12E)),
        "cmd" to FileTypeInfo("Command", Color(0xFFC1F12E)),

        // Data Serialization & Config
        "json" to FileTypeInfo("JSON", Color(0xFFCBCB41)),
        "jsonc" to FileTypeInfo("JSON with Comments", Color(0xFFCBCB41)),
        "json5" to FileTypeInfo("JSON5", Color(0xFFCBCB41)),
        "yaml" to FileTypeInfo("YAML", Color(0xFFCB171E)),
        "yml" to FileTypeInfo("YAML", Color(0xFFCB171E)),
        "toml" to FileTypeInfo("TOML", Color(0xFF9C4121)),
        "xml" to FileTypeInfo("XML", Color(0xFFE37933)),
        "ini" to FileTypeInfo("INI", Color(0xFF6D8086)),
        "env" to FileTypeInfo("ENV", Color(0xFFECD53F)),

        // Database & Query
        "sql" to FileTypeInfo("SQL", Color(0xFFE38C00)),
        "mysql" to FileTypeInfo("MySQL", Color(0xFF4479A1)),
        "pgsql" to FileTypeInfo("PostgreSQL", Color(0xFF336791)),
        "sqlite" to FileTypeInfo("SQLite", Color(0xFF003B57)),
        "graphql" to FileTypeInfo("GraphQL", Color(0xFFE10098)),
        "gql" to FileTypeInfo("GraphQL", Color(0xFFE10098)),
        "prisma" to FileTypeInfo("Prisma", Color(0xFF2D3748)),

        // Markdown & Documentation
        "md" to FileTypeInfo("Markdown", Color(0xFF4A90E2)),
        "markdown" to FileTypeInfo("Markdown", Color(0xFF4A90E2)),
        "mdx" to FileTypeInfo("MDX", Color(0xFF1B1F24)),
        "txt" to FileTypeInfo("Text", Color(0xFF888888)),
        "log" to FileTypeInfo("Log", Color(0xFF888888)),
        "pdf" to FileTypeInfo("PDF", Color(0xFFF40F02)),

        // Office & Spreadsheets
        "csv" to FileTypeInfo("CSV", Color(0xFF1D6F42)),
        "tsv" to FileTypeInfo("TSV", Color(0xFF1D6F42)),
        "xlsx" to FileTypeInfo("Excel", Color(0xFF1D6F42)),
        "xls" to FileTypeInfo("Excel", Color(0xFF1D6F42)),
        "docx" to FileTypeInfo("Word", Color(0xFF185ABD)),
        "doc" to FileTypeInfo("Word", Color(0xFF185ABD)),
        "pptx" to FileTypeInfo("PowerPoint", Color(0xFFD24726)),
        "ppt" to FileTypeInfo("PowerPoint", Color(0xFFD24726)),
        "key" to FileTypeInfo("Keynote", Color(0xFF007AFF)),

        // JVM & Functional Languages
        "scala" to FileTypeInfo("Scala", Color(0xFFDC322F)),
        "clj" to FileTypeInfo("Clojure", Color(0xFF5881D8)),
        "cljs" to FileTypeInfo("ClojureScript", Color(0xFF5881D8)),
        "groovy" to FileTypeInfo("Groovy", Color(0xFF4298B8)),
        "hs" to FileTypeInfo("Haskell", Color(0xFF5E5086)),
        "lhs" to FileTypeInfo("Haskell", Color(0xFF5E5086)),
        "ex" to FileTypeInfo("Elixir", Color(0xFF6E4A7E)),
        "exs" to FileTypeInfo("Elixir", Color(0xFF6E4A7E)),
        "erl" to FileTypeInfo("Erlang", Color(0xFFB83998)),
        "hrl" to FileTypeInfo("Erlang Header", Color(0xFFB83998)),
        "ocaml" to FileTypeInfo("OCaml", Color(0xFFEE6A1A)),
        "ml" to FileTypeInfo("OCaml", Color(0xFFEE6A1A)),
        "mli" to FileTypeInfo("OCaml Header", Color(0xFFEE6A1A)),

        // Systems & Modern Languages
        "zig" to FileTypeInfo("Zig", Color(0xFFF7A41D)),
        "nim" to FileTypeInfo("Nim", Color(0xFFFFC200)),
        "sol" to FileTypeInfo("Solidity", Color(0xFFAA6746)),
        "tf" to FileTypeInfo("Terraform", Color(0xFF844FBA)),
        "proto" to FileTypeInfo("Protobuf", Color(0xFF4F5D95)),
        "lua" to FileTypeInfo("Lua", Color(0xFF000080)),
        "r" to FileTypeInfo("R", Color(0xFF276DC3)),
        "rmd" to FileTypeInfo("R Markdown", Color(0xFF276DC3)),
        "julia" to FileTypeInfo("Julia", Color(0xFFA270BA)),
        "jl" to FileTypeInfo("Julia", Color(0xFFA270BA)),
        "v" to FileTypeInfo("V", Color(0xFF4F87C4)),
        "odin" to FileTypeInfo("Odin", Color(0xFF1389B0)),

        // Misc & Config
        "diff" to FileTypeInfo("Diff", Color(0xFF888888)),
        "patch" to FileTypeInfo("Patch", Color(0xFF888888)),
        "cmake" to FileTypeInfo("CMake", Color(0xFF064F8C)),
        "docker" to FileTypeInfo("Docker", Color(0xFF2496ED)),
        "svg" to FileTypeInfo("SVG", Color(0xFFFFB13B)),
        "tex" to FileTypeInfo("LaTeX", Color(0xFF3D6117)),
        "latex" to FileTypeInfo("LaTeX", Color(0xFF3D6117))
    )
}
