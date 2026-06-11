import AVFoundation
import Foundation

/**
 * AudioSessionManager — native iOS module for AVAudioSession configuration.
 *
 * Called by the React Native layer when a conversation session starts/ends.
 * Configures the audio session for voice chat:
 * - .playAndRecord: enables simultaneous mic input and speaker output
 * - .voiceChat mode: enables echo cancellation and noise reduction
 * - .allowBluetooth: enables BT headsets
 * - .defaultToSpeaker: falls back to loudspeaker if no headset
 *
 * Note: react-native-webrtc automatically configures AVAudioSession for basic
 * WebRTC operation. This module provides additional configuration for background
 * mode and Bluetooth fallback (REQ-B06, REQ-B09).
 */
@objc(AudioSessionManager)
class AudioSessionManager: NSObject {

  @objc
  func activate() {
    let session = AVAudioSession.sharedInstance()
    do {
      try session.setCategory(
        .playAndRecord,
        mode: .voiceChat,
        options: [.allowBluetooth, .defaultToSpeaker, .allowBluetoothA2DP]
      )
      try session.setActive(true)
    } catch {
      print("[AudioSessionManager] activate failed: \(error)")
    }
  }

  @objc
  func deactivate() {
    do {
      try AVAudioSession.sharedInstance().setActive(
        false,
        options: .notifyOthersOnDeactivation
      )
    } catch {
      print("[AudioSessionManager] deactivate failed: \(error)")
    }
  }

  @objc
  static func requiresMainQueueSetup() -> Bool {
    return false
  }
}
