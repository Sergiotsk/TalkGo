import React, { useState } from 'react';
import {
  StyleSheet,
  Text,
  TextInput,
  TouchableOpacity,
  View,
} from 'react-native';
import { useNavigation } from '@react-navigation/native';
import { NativeStackNavigationProp } from '@react-navigation/native-stack';
import { RootStackParamList } from '../navigation/types';
import { useUserStore } from '../store/userStore';

type Nav = NativeStackNavigationProp<RootStackParamList, 'Onboarding'>;

const LANGS = [
  { code: 'es', label: 'ES' },
  { code: 'pt', label: 'PT' },
  { code: 'en', label: 'EN' },
  { code: 'fr', label: 'FR' },
] as const;

const BULLETS = [
  'Traducción en tiempo real durante tu conversación',
  'Privacidad: el audio se procesa y no se almacena',
  'Sin cuenta requerida — solo tu nombre de pila',
];

export function OnboardingScreen() {
  const navigation = useNavigation<Nav>();
  const { localLang, setName, setLocalLang } = useUserStore();
  const [inputName, setInputName] = useState('');

  const canContinue = inputName.trim().length > 0;

  const handleContinue = async () => {
    if (!canContinue) return;
    await setName(inputName.trim());
    navigation.replace('Home');
  };

  return (
    <View style={styles.container}>
      <Text style={styles.title}>Bienvenido a TalkGo</Text>
      <Text style={styles.subtitle}>Hablá con cualquiera, en su idioma.</Text>

      <View style={styles.bullets}>
        {BULLETS.map((b) => (
          <Text key={b} style={styles.bullet}>
            {'• '}{b}
          </Text>
        ))}
      </View>

      <Text style={styles.label}>Tu idioma</Text>
      <View style={styles.langRow}>
        {LANGS.map(({ code, label }) => (
          <TouchableOpacity
            key={code}
            style={[styles.langBtn, localLang === code && styles.langBtnActive]}
            onPress={() => setLocalLang(code)}
          >
            <Text style={[styles.langText, localLang === code && styles.langTextActive]}>
              {label}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      <Text style={styles.label}>Tu nombre</Text>
      <TextInput
        style={styles.input}
        placeholder="Nombre"
        placeholderTextColor="#888"
        value={inputName}
        onChangeText={setInputName}
        autoCapitalize="words"
        returnKeyType="done"
      />

      <TouchableOpacity
        style={[styles.btn, !canContinue && styles.btnDisabled]}
        onPress={handleContinue}
        disabled={!canContinue}
      >
        <Text style={styles.btnText}>Continuar</Text>
      </TouchableOpacity>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0f0f0f',
    padding: 24,
    justifyContent: 'center',
  },
  title: {
    color: '#ffffff',
    fontSize: 26,
    fontWeight: '700',
    marginBottom: 8,
    textAlign: 'center',
  },
  subtitle: {
    color: '#aaaaaa',
    fontSize: 15,
    textAlign: 'center',
    marginBottom: 32,
  },
  bullets: {
    marginBottom: 32,
    gap: 10,
  },
  bullet: {
    color: '#cccccc',
    fontSize: 14,
    lineHeight: 20,
  },
  label: {
    color: '#888888',
    fontSize: 12,
    textTransform: 'uppercase',
    letterSpacing: 1,
    marginBottom: 10,
  },
  langRow: {
    flexDirection: 'row',
    gap: 10,
    marginBottom: 28,
  },
  langBtn: {
    borderWidth: 1,
    borderColor: '#333',
    borderRadius: 8,
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  langBtnActive: {
    borderColor: '#4caf50',
    backgroundColor: '#1a2e1a',
  },
  langText: {
    color: '#aaaaaa',
    fontSize: 14,
    fontWeight: '600',
  },
  langTextActive: {
    color: '#4caf50',
  },
  input: {
    backgroundColor: '#1a1a1a',
    borderWidth: 1,
    borderColor: '#333',
    borderRadius: 10,
    padding: 14,
    color: '#ffffff',
    fontSize: 16,
    marginBottom: 28,
  },
  btn: {
    backgroundColor: '#4caf50',
    borderRadius: 12,
    padding: 16,
    alignItems: 'center',
  },
  btnDisabled: {
    backgroundColor: '#2a4a2a',
    opacity: 0.6,
  },
  btnText: {
    color: '#ffffff',
    fontSize: 16,
    fontWeight: '700',
  },
});
