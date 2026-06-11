// TASK-084: Tests for api.ts HTTP client

import { createRoom, findRoomByCode, deleteRoom, ApiError } from '../../src/services/api';

// Mock fetch globally
const mockFetch = jest.fn();
global.fetch = mockFetch;

describe('api.ts', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  describe('createRoom', () => {
    it('makes POST /rooms and returns room_id and short_code', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 201,
        json: () =>
          Promise.resolve({ room_id: 'uuid-123', short_code: 'ABC123' }),
      });

      const result = await createRoom('es', 'en');

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/rooms'),
        expect.objectContaining({ method: 'POST' })
      );
      expect(result.room_id).toBe('uuid-123');
      expect(result.short_code).toBe('ABC123');
    });

    it('throws ApiError on non-OK response', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 500,
        text: () => Promise.resolve('internal error'),
      });

      await expect(createRoom('es', 'en')).rejects.toThrow(ApiError);
    });
  });

  describe('findRoomByCode', () => {
    it('GET /rooms/code/{code} returns room on 200', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({ room_id: 'uuid-456', short_code: 'XYZ789' }),
      });

      const result = await findRoomByCode('XYZ789');

      expect(result.room_id).toBe('uuid-456');
    });

    it('throws ApiError(404) when room not found', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        text: () => Promise.resolve('not found'),
      });

      const error = await findRoomByCode('XXXXXX').catch((e: unknown) => e);
      expect(error).toBeInstanceOf(ApiError);
      expect((error as ApiError).statusCode).toBe(404);
    });

    it('throws ApiError(410) when room expired', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 410,
        text: () => Promise.resolve('expired'),
      });

      const error = await findRoomByCode('OLDONE').catch((e: unknown) => e);
      expect(error).toBeInstanceOf(ApiError);
      expect((error as ApiError).statusCode).toBe(410);
      expect((error as ApiError).message).toMatch(/expiró/);
    });
  });

  describe('deleteRoom', () => {
    it('DELETE /rooms/{id} succeeds on 200', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        text: () => Promise.resolve(''),
      });

      await expect(deleteRoom('uuid-123')).resolves.toBeUndefined();
    });

    it('silently succeeds on 404 (room already gone)', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        text: () => Promise.resolve('not found'),
      });

      await expect(deleteRoom('uuid-gone')).resolves.toBeUndefined();
    });
  });
});
