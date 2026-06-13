import React, { useEffect } from 'react';
import { createNativeStackNavigator } from '@react-navigation/native-stack';
import { HomeScreen } from '../screens/HomeScreen';
import { OnboardingScreen } from '../screens/OnboardingScreen';
import { ConversationScreen } from '../screens/ConversationScreen';
import { useUserStore } from '../store/userStore';
import { RootStackParamList } from './types';

const Stack = createNativeStackNavigator<RootStackParamList>();

export function RootNavigator() {
  const { name, hydrate } = useUserStore();

  useEffect(() => {
    hydrate();
  }, [hydrate]);

  const initialRoute: keyof RootStackParamList = name ? 'Home' : 'Onboarding';

  return (
    <Stack.Navigator
      initialRouteName={initialRoute}
      screenOptions={{ headerShown: false }}
    >
      <Stack.Screen name="Onboarding" component={OnboardingScreen} />
      <Stack.Screen name="Home" component={HomeScreen} />
      <Stack.Screen name="Conversation" component={ConversationScreen as React.ComponentType} />
    </Stack.Navigator>
  );
}
