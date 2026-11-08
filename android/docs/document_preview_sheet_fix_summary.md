# DocumentPreviewSheet.kt Compilation Error Summary & Technical Reference

## Bug Overview
- **Error Message**: `Unresolved reference 'p'` during Kotlin compilation.
- **Affected File**: `app/src/main/java/com/antigravity/mobile/ui/components/DocumentPreviewSheet.kt` (around line 216).

## Root Cause Analysis
1. **Kotlin Triple-Quoted String Interpolation (`""" ... """`)**: 
   - In Kotlin multi-line strings, the dollar sign (`$`) is reserved for string interpolation (e.g., `$variable` or `${expression}`).
   - The CSS injection snippet for mobile adaptation of Marp presentations contained a CSS selector matching a DOM element ID with a colon and dollar sign: `div#\:\$p`.
   - The Kotlin compiler interpreted `\$p` (or `$p`) inside the triple-quoted string literal as an attempt to interpolate a variable named `p`, which was not declared in scope, leading to the `Unresolved reference 'p'` error.

## Resolution & Best Practices
- **Escaping Dollar Signs in Kotlin Multi-line Strings**:
  - Replace literal dollar signs that should not be interpolated with `${'$'}`.
  - In our case, `div#\:\$p` was updated to `div#\:\${'$'}p`.
- **Key Takeaway for Agents**: When injecting CSS or JavaScript containing CSS pseudo-classes, IDs, or variables with `$` (like CSS variables `--${'$'}` or DOM IDs with `$`) inside Kotlin triple-quoted strings (`"""`), always escape the dollar sign using `${'$'}` to prevent compilation errors.
