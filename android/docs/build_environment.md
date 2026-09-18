# Android Build Environment & Toolchain Specification

This document defines the official build environment, toolchain versions, and configuration standards for the `antigravity-mobile` Android application.

## 🛠 Core Toolchain Versions

| Component | Version | Description |
| :--- | :--- | :--- |
| **Gradle** | `9.7.1` | Gradle build system (`gradle-9.7.1-bin.zip`) |
| **Android Gradle Plugin (AGP)** | `8.8.2` | Android build plugin for Gradle |
| **Kotlin** | `2.0.21` | Kotlin programming language |
| **Kotlin Serialization** | `2.0.21` | Kotlin serialization plugin |
| **Compose Compiler Plugin** | `2.0.21` | Jetpack Compose compiler plugin |
| **Java / JVM Target** | `17` | Source and Target compatibility |

---

## 📱 Android SDK Configurations

| Setting | Value |
| :--- | :--- |
| **Namespace** | `com.antigravity.mobile` |
| **Compile SDK** | `34` |
| **Target SDK** | `34` |
| **Min SDK** | `26` (Android 8.0 Oreo) |
| **Application ID** | `com.antigravity.mobile` |
| **Version Code** | `1` |
| **Version Name** | `1.0.0` |

---

## 🎨 UI & Framework Libraries

- **Jetpack Compose BOM**: `2024.04.01`
- **Compose Material 3**: `1.2.1`
- **Navigation Compose**: `2.7.7`
- **Lifecycle & ViewModel**: `2.7.0`
- **Coil Compose (Image Loading & SVG)**: `2.6.0`

---

## 🌐 Networking & Utilities

- **OkHttp & Logging Interceptor**: `4.12.0`
- **Kotlinx Serialization JSON**: `1.6.3`
- **Kotlinx Coroutines Android**: `1.8.0`
- **Security Crypto**: `1.1.0-alpha06`
- **ZXing Android Embedded (QR Code)**: `4.3.0`

---

## 🚀 Build & Compilation Instructions

To build the project across multiple environments (local development, CI/CD, etc.), ensure you use the project wrapper:

```bash
# Clean build
./gradlew clean

# Build Debug APK
./gradlew :app:assembleDebug

# Build Release APK / Bundle
./gradlew :app:assembleRelease
```
