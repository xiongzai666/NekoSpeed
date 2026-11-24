import java.io.FileInputStream
import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val keystoreProperties = Properties().apply {
    val f = rootProject.file("keystore.properties")
    if (f.exists()) {
        FileInputStream(f).use { load(it) }
    }
}

val propStoreFile = keystoreProperties.getProperty("storeFile")?.trim().orEmpty()
val propStorePass = keystoreProperties.getProperty("storePassword").orEmpty()
val propKeyAlias = keystoreProperties.getProperty("keyAlias")?.trim().orEmpty()
val propKeyPass = keystoreProperties.getProperty("keyPassword").orEmpty()

val envStorePath = System.getenv("ANDROID_KEYSTORE_PATH")?.trim().orEmpty()
val envStorePass = System.getenv("ANDROID_KEYSTORE_PASSWORD").orEmpty()
val envKeyAlias = System.getenv("ANDROID_KEY_ALIAS")?.trim().orEmpty()
val envKeyPass = System.getenv("ANDROID_KEY_PASSWORD").orEmpty()

val hasPropSigning = propStoreFile.isNotEmpty() &&
    propStorePass.isNotEmpty() &&
    propKeyAlias.isNotEmpty() &&
    propKeyPass.isNotEmpty() &&
    rootProject.file(propStoreFile).exists()

val hasEnvSigning = envStorePath.isNotEmpty() &&
    envStorePass.isNotEmpty() &&
    envKeyAlias.isNotEmpty() &&
    envKeyPass.isNotEmpty() &&
    file(envStorePath).exists()

val hasReleaseSigning = hasPropSigning || hasEnvSigning

val configuredAbiFilters = (findProperty("abiFilters") as? String)
    ?.split(',')
    ?.map { it.trim() }
    ?.filter { it.isNotEmpty() }
    ?.takeIf { it.isNotEmpty() }
    ?: listOf("arm64-v8a")

val configuredVersionCode = (findProperty("versionCode") as? String)
    ?.trim()
    ?.toIntOrNull()
    ?: 3

val configuredVersionName = (findProperty("versionName") as? String)
    ?.trim()
    ?.takeIf { it.isNotEmpty() }
    ?: "0.3.0"

android {
    namespace = "com.nekospeed.android"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.nekospeed.android"
        minSdk = 26
        targetSdk = 35
        versionCode = configuredVersionCode
        versionName = configuredVersionName

        ndk {
            abiFilters.addAll(configuredAbiFilters)
        }
    }

    signingConfigs {
        if (hasReleaseSigning) {
            create("release") {
                if (hasPropSigning) {
                    storeFile = rootProject.file(propStoreFile)
                    storePassword = propStorePass
                    keyAlias = propKeyAlias
                    keyPassword = propKeyPass
                } else {
                    storeFile = file(envStorePath)
                    storePassword = envStorePass
                    keyAlias = envKeyAlias
                    keyPassword = envKeyPass
                }
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            signingConfig = if (hasReleaseSigning) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.activity:activity-ktx:1.9.3")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")

    testImplementation("junit:junit:4.13.2")
}
