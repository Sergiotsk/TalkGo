export type RootStackParamList = {
  Onboarding: undefined;
  Home: undefined;
  Conversation: {
    roomId: string;
    shortCode: string;
    userId: string;
    serverUrl: string;
    localLang: string;
    peerLang: string;
  };
};
