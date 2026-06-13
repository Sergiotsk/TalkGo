/**
 * TalkGo Mobile — Entry point (App.tsx)
 *
 * Sprint 3: Mounts ConversationScreen with test props.
 * In a production app, this would be a navigation stack (React Navigation).
 * For Sprint 3, the single-screen architecture is intentional — routing is Sprint 4 scope.
 */

import React from 'react';
import { ConversationScreen } from './src/screens/ConversationScreen';

function App(): React.JSX.Element {
  // Sprint 3: hard-coded test props — room/user lookup will be Sprint 4 (navigation).
  // To test with a real backend:
  //   1. POST /rooms → get room_id + short_code
  //   2. Pass room_id and wss://HOST as serverUrl
  return (
    <ConversationScreen
      roomId="test-room-id"
      shortCode="TEST01"
      userId="user-dev"
      serverUrl="wss://138-201-95-167.sslip.io"
      localLang="es"
      peerLang="en"
    />
  );
}

export default App;
