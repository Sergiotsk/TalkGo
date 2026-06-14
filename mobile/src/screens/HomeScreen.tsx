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
import { CreateRoomResponse, createRoom, findRoomByCode } from '../services/api';
import { RootStackParamList } from '../navigation/types';
import { useUserStore } from '../store/userStore';

type Nav = NativeStackNavigationProp<RootStackParamList, 'Home'>;

const SERVER_URL = 'wss://138-201-95-167.sslip.io';

function randomUserId(): string {
  return Math.random().toString(36).slice(2, 10);
}

const PEER_LANGS = [
  { code: 'es', label: 'ES' },
  { code: 'pt', label: 'PT' },
  { code: 'en', label: 'EN' },
  { code: 'fr', label: 'FR' },
] as const;

export function HomeScreen() {
  const navigation = useNavigation<Nav>();
  const { name, localLang } = useUserStore();

  // — Crear sala state —
  const [peerLang, setPeerLang] = useState('en');
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');
  const [createdRoom, setCreatedRoom] = useState<CreateRoomResponse | null>(null);

  // — Unirse state —
  const [joinCode, setJoinCode] = useState('');
  const [joining, setJoining] = useState(false);
  const [joinError, setJoinError] = useState('');

  const handleCreate = async () => {
    setCreating(true);
    setCreateError('');
    setCreatedRoom(null);
    try {
      const room = await createRoom(localLang, peerLang);
      setCreatedRoom(room);
    } catch (e) {
      console.error('[HomeScreen] createRoom failed:', e);
      setCreateError('Error al crear la sala. Intentá de nuevo.');
    } finally {
      setCreating(false);
    }
  };

  const handleJoinCreated = () => {
    if (!createdRoom) return;
    navigation.navigate('Conversation', {
      roomId: createdRoom.room_id,
      shortCode: createdRoom.short_code,
      userId: randomUserId(),
      serverUrl: SERVER_URL,
      localLang,
      peerLang: 'auto',
    });
  };

  const handleJoin = async () => {
    if (joinCode.length < 6) return;
    setJoining(true);
    setJoinError('');
    try {
      const room = await findRoomByCode(joinCode.toUpperCase());
      navigation.navigate('Conversation', {
        roomId: room.room_id,
        shortCode: room.short_code ?? joinCode.toUpperCase(),
        userId: randomUserId(),
        serverUrl: SERVER_URL,
        localLang,
        peerLang: room.peer_lang ?? 'en',
      });
    } catch (e) {
      const err = e as { message?: string };
      setJoinError(err.message ?? 'Error al unirse. Intentá de nuevo.');
    } finally {
      setJoining(false);
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.greeting}>Hola, {name}</Text>

      {/* — Crear sala — */}
      <View style={styles.card}>
        <Text style={styles.cardTitle}>Nueva conversación</Text>
        <Text style={styles.cardDesc}>Creá una sala y compartí el código con tu interlocutor.</Text>

        <Text style={styles.label}>Idioma del otro</Text>
        <View style={styles.langRow}>
          {PEER_LANGS.map(({ code, label }) => (
            <TouchableOpacity
              key={code}
              style={[styles.langBtn, peerLang === code && styles.langBtnActive]}
              onPress={() => setPeerLang(code)}
            >
              <Text style={[styles.langText, peerLang === code && styles.langTextActive]}>
                {label}
              </Text>
            </TouchableOpacity>
          ))}
        </View>

        {createdRoom ? (
          <View style={styles.codeBox}>
            <Text style={styles.codeLabel}>Código de sala</Text>
            <Text style={styles.code}>{createdRoom.short_code}</Text>
            <TouchableOpacity style={styles.btn} onPress={handleJoinCreated}>
              <Text style={styles.btnText}>Unirme a mi sala</Text>
            </TouchableOpacity>
          </View>
        ) : (
          <TouchableOpacity
            style={[styles.btn, creating && styles.btnDisabled]}
            onPress={handleCreate}
            disabled={creating}
          >
            <Text style={styles.btnText}>{creating ? 'Creando sala...' : 'Crear sala'}</Text>
          </TouchableOpacity>
        )}

        {createError ? <Text style={styles.error}>{createError}</Text> : null}
      </View>

      {/* — Unirse — */}
      <View style={styles.card}>
        <Text style={styles.cardTitle}>Unirse a una sala</Text>
        <Text style={styles.cardDesc}>Ingresá el código de 6 letras que te compartieron.</Text>

        <TextInput
          style={styles.input}
          placeholder="Código de 6 letras"
          placeholderTextColor="#888"
          value={joinCode}
          onChangeText={(t) => setJoinCode(t.toUpperCase())}
          maxLength={6}
          autoCapitalize="characters"
          autoCorrect={false}
        />

        <TouchableOpacity
          style={[styles.btn, (joinCode.length < 6 || joining) && styles.btnDisabled]}
          onPress={handleJoin}
          disabled={joinCode.length < 6 || joining}
        >
          <Text style={styles.btnText}>{joining ? 'Buscando sala...' : 'Unirse'}</Text>
        </TouchableOpacity>

        {joinError ? <Text style={styles.error}>{joinError}</Text> : null}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0f0f0f',
    padding: 20,
  },
  greeting: {
    color: '#ffffff',
    fontSize: 22,
    fontWeight: '700',
    marginTop: 16,
    marginBottom: 24,
  },
  card: {
    backgroundColor: '#1a1a1a',
    borderRadius: 14,
    padding: 20,
    marginBottom: 16,
  },
  cardTitle: {
    color: '#ffffff',
    fontSize: 16,
    fontWeight: '700',
    marginBottom: 6,
  },
  cardDesc: {
    color: '#888',
    fontSize: 13,
    marginBottom: 16,
  },
  label: {
    color: '#888888',
    fontSize: 11,
    textTransform: 'uppercase',
    letterSpacing: 1,
    marginBottom: 8,
  },
  langRow: {
    flexDirection: 'row',
    gap: 8,
    marginBottom: 14,
  },
  langBtn: {
    borderWidth: 1,
    borderColor: '#333',
    borderRadius: 8,
    paddingHorizontal: 14,
    paddingVertical: 6,
  },
  langBtnActive: {
    borderColor: '#4caf50',
    backgroundColor: '#1a2e1a',
  },
  langText: {
    color: '#aaaaaa',
    fontSize: 13,
    fontWeight: '600',
  },
  langTextActive: {
    color: '#4caf50',
  },
  codeBox: {
    alignItems: 'center',
    gap: 12,
  },
  codeLabel: {
    color: '#888',
    fontSize: 12,
    textTransform: 'uppercase',
    letterSpacing: 1,
  },
  code: {
    color: '#4caf50',
    fontSize: 36,
    fontWeight: '800',
    letterSpacing: 6,
  },
  input: {
    backgroundColor: '#0f0f0f',
    borderWidth: 1,
    borderColor: '#333',
    borderRadius: 10,
    padding: 14,
    color: '#ffffff',
    fontSize: 20,
    fontWeight: '700',
    letterSpacing: 4,
    textAlign: 'center',
    marginBottom: 12,
  },
  btn: {
    backgroundColor: '#4caf50',
    borderRadius: 10,
    padding: 14,
    alignItems: 'center',
  },
  btnDisabled: {
    backgroundColor: '#2a4a2a',
    opacity: 0.6,
  },
  btnText: {
    color: '#ffffff',
    fontSize: 15,
    fontWeight: '700',
  },
  error: {
    color: '#ef5350',
    fontSize: 13,
    marginTop: 10,
    textAlign: 'center',
  },
});
