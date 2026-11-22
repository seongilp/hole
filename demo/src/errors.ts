/** Errors with an HTTP status and a safe, user-facing Korean message. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export const MSG = {
  badBody: '요청 형식이 올바르지 않습니다.',
  tooLarge: '요청이 너무 큽니다.',
  forbiddenOrigin: '허용되지 않은 출처입니다.',
  notFound: '찾을 수 없는 경로입니다.',
  rateLimited: '요청이 너무 많습니다. 잠시 후 다시 시도해 주세요.',
  quota: '오늘 데모 사용량이 다 찼습니다. 아래 예시 결과를 보거나 내일 다시 시도해 주세요.',
  timeout: '페이지를 불러오는 데 시간이 너무 오래 걸립니다.',
  loadFailed: '페이지를 열 수 없습니다. 주소를 확인해 주세요.',
  internal: '검사 중 문제가 생겼습니다. 잠시 후 다시 시도해 주세요.',
} as const;

export const apiError = (status: number, message: string): ApiError => new ApiError(status, message);
