// HTTP client for the TalkGo Go backend.
// BASE_URL can be overridden via environment variable or configuration.

const BASE_URL = 'https://138-201-95-167.sslip.io';

export class ApiError extends Error {
  constructor(
    public statusCode: number,
    message: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export interface CreateRoomResponse {
  room_id: string;
  short_code: string;
}

export interface FindRoomResponse {
  room_id: string;
  short_code?: string;
}

/**
 * createRoom — POST /rooms
 * Creates a new room and returns its ID and short code.
 */
export async function createRoom(
  sourceLang: string,
  targetLang: string
): Promise<CreateRoomResponse> {
  const response = await fetch(`${BASE_URL}/rooms`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source_lang: sourceLang, target_lang: targetLang }),
  });

  if (!response.ok) {
    const text = await response.text();
    throw new ApiError(response.status, text);
  }

  return response.json() as Promise<CreateRoomResponse>;
}

/**
 * findRoomByCode — GET /rooms/code/{code}
 * Looks up a room by its 6-char short code.
 * Throws ApiError(404) if not found, ApiError(410) if expired.
 */
export async function findRoomByCode(code: string): Promise<FindRoomResponse> {
  const response = await fetch(
    `${BASE_URL}/rooms/code/${encodeURIComponent(code)}`
  );

  if (response.status === 404) {
    throw new ApiError(404, 'Sala no encontrada. Verificá el código.');
  }

  if (response.status === 410) {
    throw new ApiError(410, 'Esta sala expiró. Creá una nueva.');
  }

  if (!response.ok) {
    const text = await response.text();
    throw new ApiError(response.status, text);
  }

  return response.json() as Promise<FindRoomResponse>;
}

/**
 * deleteRoom — DELETE /rooms/{roomId}
 * Deletes a room. Used for cleanup after voluntary session end.
 */
export async function deleteRoom(roomId: string): Promise<void> {
  const response = await fetch(`${BASE_URL}/rooms/${encodeURIComponent(roomId)}`, {
    method: 'DELETE',
  });

  if (!response.ok && response.status !== 404) {
    const text = await response.text();
    throw new ApiError(response.status, text);
  }
}
