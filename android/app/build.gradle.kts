import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.serialization")
    id("org.jetbrains.kotlin.plugin.compose")
}

val localProps = Properties().apply {
    val f = rootProject.file("local.properties")
    if (f.exists()) {
        f.inputStream().use { load(it) }
    }
}

fun resolveSigningValue(envKey: String, propKey: String): String? {
    return System.getenv(envKey)?.takeIf { it.isNotBlank() }
        ?: localProps.getProperty(propKey)?.takeIf { it.isNotBlank() }
}

val releaseKeystorePath = resolveSigningValue("MGY_KEYSTORE_PATH", "mgy.keystore.path")
val releaseKeystorePassword = resolveSigningValue("MGY_KEYSTORE_PASSWORD", "mgy.keystore.password")
val releaseKeyAlias = resolveSigningValue("MGY_KEY_ALIAS", "mgy.key.alias")
val releaseKeyPassword = resolveSigningValue("MGY_KEY_PASSWORD", "mgy.key.password")
val releaseKeystoreFile = releaseKeystorePath?.let { file(it) }?.takeIf { it.exists() }
val hasReleaseSigning = releaseKeystoreFile != null &&
    !releaseKeystorePassword.isNullOrBlank() &&
    !releaseKeyAlias.isNullOrBlank() &&
    !releaseKeyPassword.isNullOrBlank()

android {
    namespace = "com.antigravity.mobile"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.antigravity.mobile"
        minSdk = 26
        targetSdk = 34
        versionCode = 39
        versionName = "1.2.7"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        vectorDrawables {
            useSupportLibrary = true
        }
    }

    flavorDimensions += "distribution"
    productFlavors {
        create("direct") {
            dimension = "distribution"
            isDefault = true
            // GitHub Releases / Official Website / Obtainium build:
            // Keeps legacy applicationId and enables restrained in-app self-update.
            buildConfigField("boolean", "ENABLE_SELF_UPDATE", "true")
            buildConfigField("String", "DISTRIBUTION_CHANNEL", "\"direct\"")
        }
        create("play") {
            dimension = "distribution"
            // Reserved for future Google Play Store publishing:
            // - Distinct applicationId to avoid trademark collision & separate Play App Signing.
            // - Zero REQUEST_INSTALL_PACKAGES permission and zero self-update APK installer code.
            applicationId = "com.ghsaimo.multigravity"
            buildConfigField("boolean", "ENABLE_SELF_UPDATE", "false")
            buildConfigField("String", "DISTRIBUTION_CHANNEL", "\"play\"")
        }
    }

    signingConfigs {
        create("release") {
            if (hasReleaseSigning) {
                storeFile = releaseKeystoreFile
                storePassword = releaseKeystorePassword
                keyAlias = releaseKeyAlias
                keyPassword = releaseKeyPassword
            }
        }
    }

    buildTypes {
        release {
            if (hasReleaseSigning) {
                signingConfig = signingConfigs.getByName("release")
            }
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
    testOptions {
        unitTests.isReturnDefaultValues = true
    }
    lint {
        abortOnError = false
        checkReleaseBuilds = false
        disable += "InvalidFragmentVersionForActivityResult"
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2024.04.01")
    implementation(composeBom)
    androidTestImplementation(composeBom)

    implementation("androidx.core:core-ktx:1.12.0")
    implementation("androidx.fragment:fragment-ktx:1.6.2")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.7.0")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.7.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.7.0")
    implementation("androidx.activity:activity-compose:1.8.2")

    // Jetpack Compose UI & Material 3
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-graphics")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.compose.material3:material3:1.3.1")
    implementation("androidx.compose.material:material-icons-extended")

    // Navigation Compose
    implementation("androidx.navigation:navigation-compose:2.7.7")

    // Image Loading (Coil Compose + SVG Decoder)
    implementation("io.coil-kt:coil-compose:2.6.0")
    implementation("io.coil-kt:coil-svg:2.6.0")
    implementation("com.google.zxing:core:3.5.3")

    // Network: OkHttp 4.12.0 + WebSocket
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("com.squareup.okhttp3:logging-interceptor:4.12.0")

    // Serialization
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.6.3")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.0")

    // Security (Android Keystore / EncryptedSharedPreferences)
    implementation("androidx.security:security-crypto:1.1.0-alpha06")

    // QR Code Scanning (ZXing embedded, zero-GMS dependency)
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")

    // ML Kit Document Scanner (Dynamic Google Play Services capability)
    implementation("com.google.android.gms:play-services-mlkit-document-scanner:16.0.0")

    // Push Notifications (Firebase Cloud Messaging)
    implementation("com.google.firebase:firebase-messaging:23.4.1")

    // In-App Browser (Chrome Custom Tabs)
    implementation("androidx.browser:browser:1.8.0")

    // Testing
    testImplementation("junit:junit:4.13.2")
    androidTestImplementation("androidx.test.ext:junit:1.1.5")
    androidTestImplementation("androidx.test.espresso:espresso-core:3.5.1")
    debugImplementation("androidx.compose.ui:ui-tooling")
    debugImplementation("androidx.compose.ui:ui-test-manifest")
}

// Fail fast if packaging a release APK/AAB without the private signing keystore configured,
// while still allowing open-source contributors and CI PR checks to run compileReleaseKotlin / unit tests.
gradle.taskGraph.whenReady {
    val requiresReleaseSigning = allTasks.any { task ->
        val name = task.name
        (name.startsWith("assemble") ||
            name.startsWith("bundle") ||
            name.startsWith("package") ||
            name.startsWith("sign")) &&
            (name.endsWith("Release") || name.endsWith("ReleaseBundle"))
    }
    if (requiresReleaseSigning && !hasReleaseSigning) {
        throw GradleException(
            "❌ 致命错误: 未配置私有 Release 签名密钥库！禁止静默产出未签名或 debug 签名发包。\n" +
                "请通过环境变量 (MGY_KEYSTORE_PATH / MGY_KEYSTORE_PASSWORD / MGY_KEY_ALIAS / MGY_KEY_PASSWORD) " +
                "或本地 android/local.properties (mgy.keystore.path / mgy.keystore.password / mgy.key.alias / mgy.key.password) 提供私有 keystore。"
        )
    }
}

// Preserve the standard `./gradlew compileReleaseKotlin` verification gate across both flavors.
tasks.register("compileReleaseKotlin") {
    group = "verification"
    description = "Compiles Release Kotlin sources for both direct and play distribution flavors."
    dependsOn("compileDirectReleaseKotlin", "compilePlayReleaseKotlin")
}
