package com.talkgomobile

import android.content.Intent
import androidx.core.content.ContextCompat
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod

/**
 * CallServiceModule — React Native NativeModules bridge for CallForegroundService.
 *
 * Exposed as NativeModules.CallService in TypeScript.
 * Usage:
 *   import { NativeModules } from 'react-native';
 *   NativeModules.CallService.start();
 *   NativeModules.CallService.stop();
 *
 * Must be registered in MainApplication via a ReactPackage.
 */
class CallServiceModule(reactContext: ReactApplicationContext) :
    ReactContextBaseJavaModule(reactContext) {

    override fun getName(): String = "CallService"

    @ReactMethod
    fun start() {
        val intent = Intent(reactApplicationContext, CallForegroundService::class.java)
        ContextCompat.startForegroundService(reactApplicationContext, intent)
    }

    @ReactMethod
    fun stop() {
        val intent = Intent(reactApplicationContext, CallForegroundService::class.java)
        reactApplicationContext.stopService(intent)
    }
}
