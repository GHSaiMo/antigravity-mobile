# ==============================================================================
# General Attributes & Debugging Info
# ==============================================================================
-keepattributes *Annotation*,Signature,InnerClasses,EnclosingMethod
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile

# ==============================================================================
# Android WebKit / JavaScriptInterface (Mermaid Diagram Rendering)
# ==============================================================================
-keepattributes JavascriptInterface
-keepclassmembers class * {
    @android.webkit.JavascriptInterface <methods>;
}

# ==============================================================================
# kotlinx.serialization & Data Models
# Keep model classes and member fields to guarantee 1:1 match with Gateway JSON
# ==============================================================================
-keep class com.antigravity.mobile.data.model.** { *; }
-keep @kotlinx.serialization.Serializable class * { *; }

-keepclassmembers class * {
    public static final ** Companion;
}

-keepnames class *$$serializer { *; }
-keepclassmembers class *$$serializer {
    public static final *** INSTANCE;
}

# ==============================================================================
# AndroidX Security Crypto (Google Tink & Protobuf Lite)
# Essential for EncryptedSharedPreferences to avoid ClassNotFoundException on launch
# ==============================================================================
-keep class com.google.crypto.tink.** { *; }
-dontwarn com.google.crypto.tink.**
-keepclassmembers class * extends com.google.crypto.tink.shaded.protobuf.GeneratedMessageLite {
    *;
}
-keep class com.google.crypto.tink.shaded.protobuf.** { *; }
-dontwarn com.google.crypto.tink.shaded.protobuf.**
-keep class com.google.protobuf.** { *; }
-dontwarn com.google.protobuf.**

# ==============================================================================
# Network: OkHttp 4.12.0 & Okio & WebSockets
# ==============================================================================
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**
-dontwarn okhttp3.**
-dontwarn okio.**
-keep class okhttp3.** { *; }
-keep interface okhttp3.** { *; }

# ==============================================================================
# Kotlin Coroutines
# ==============================================================================
-keepnames class kotlinx.coroutines.internal.MainDispatcherFactory {}
-keepnames class kotlinx.coroutines.CoroutineExceptionHandler {}
-dontwarn kotlinx.coroutines.**

# ==============================================================================
# ZXing Android Embedded (QR Code Scanner)
# ==============================================================================
-keep class com.journeyapps.barcodescanner.** { *; }
-dontwarn com.journeyapps.barcodescanner.**
-keep class com.google.zxing.** { *; }
-dontwarn com.google.zxing.**

# ==============================================================================
# Firebase Cloud Messaging
# ==============================================================================
-keep class com.google.firebase.messaging.** { *; }
-dontwarn com.google.firebase.messaging.**
-keep public class * extends com.google.firebase.messaging.FirebaseMessagingService { *; }

# ==============================================================================
# Image Loading: Coil & SVG Decoder
# ==============================================================================
-dontwarn coil.**
