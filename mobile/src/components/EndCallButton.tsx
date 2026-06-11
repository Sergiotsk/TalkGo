import React, { useState } from 'react';
import { Modal, StyleSheet, Text, TouchableOpacity, View } from 'react-native';

export interface EndCallButtonProps {
  onConfirm: () => void;
}

/**
 * EndCallButton — "Finalizar" button with a confirmation Modal.
 * Pressing confirm calls onConfirm(); pressing cancel closes the modal.
 */
export function EndCallButton({ onConfirm }: EndCallButtonProps): React.JSX.Element {
  const [visible, setVisible] = useState(false);

  const handlePress = () => setVisible(true);

  const handleCancel = () => setVisible(false);

  const handleConfirm = () => {
    setVisible(false);
    onConfirm();
  };

  return (
    <>
      <TouchableOpacity
        style={styles.endButton}
        onPress={handlePress}
        accessibilityLabel="Finalizar conversación"
        accessibilityRole="button"
      >
        <Text style={styles.endButtonText}>Finalizar</Text>
      </TouchableOpacity>

      <Modal
        visible={visible}
        transparent
        animationType="fade"
        onRequestClose={handleCancel}
      >
        <View style={styles.overlay}>
          <View style={styles.dialog}>
            <Text style={styles.dialogTitle}>¿Terminar conversación?</Text>
            <Text style={styles.dialogMessage}>
              Se desconectará de la sesión activa.
            </Text>
            <View style={styles.dialogButtons}>
              <TouchableOpacity
                style={[styles.dialogButton, styles.cancelButton]}
                onPress={handleCancel}
              >
                <Text style={styles.cancelButtonText}>Cancelar</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={[styles.dialogButton, styles.confirmButton]}
                onPress={handleConfirm}
              >
                <Text style={styles.confirmButtonText}>Confirmar</Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  endButton: {
    backgroundColor: '#F44336',
    borderRadius: 40,
    paddingHorizontal: 32,
    paddingVertical: 14,
    alignItems: 'center',
  },
  endButtonText: {
    color: '#fff',
    fontSize: 16,
    fontWeight: '600',
  },
  overlay: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.5)',
    justifyContent: 'center',
    alignItems: 'center',
  },
  dialog: {
    backgroundColor: '#fff',
    borderRadius: 12,
    padding: 24,
    width: '80%',
    maxWidth: 360,
  },
  dialogTitle: {
    fontSize: 18,
    fontWeight: '700',
    color: '#333',
    marginBottom: 8,
  },
  dialogMessage: {
    fontSize: 14,
    color: '#666',
    marginBottom: 20,
  },
  dialogButtons: {
    flexDirection: 'row',
    justifyContent: 'flex-end',
    gap: 12,
  },
  dialogButton: {
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 6,
  },
  cancelButton: {
    backgroundColor: '#E0E0E0',
  },
  cancelButtonText: {
    color: '#333',
    fontWeight: '500',
  },
  confirmButton: {
    backgroundColor: '#F44336',
  },
  confirmButtonText: {
    color: '#fff',
    fontWeight: '600',
  },
});
