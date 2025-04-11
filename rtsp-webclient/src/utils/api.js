const API_BASE_URL = "http://localhost:8080"; // Базовый URL сервера

// Функция для проверки ответа от сервера
const handleResponse = async (response) => {
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(errorText || `HTTP error! Status: ${response.status}`);
  }
  return response.json();
};

// Получить список активных стримов
export const getStreams = async () => {
  try {
    const response = await fetch(`${API_BASE_URL}/list-streams`);
    return handleResponse(response);
  } catch (error) {
    console.error("Error fetching streams:", error.message);
    throw new Error("Failed to fetch active streams");
  }
};

// Получить список архивных стримов
export const getArchivedStreams = async () => {
  try {
    const response = await fetch(`${API_BASE_URL}/archive/list`);
    return handleResponse(response);
  } catch (error) {
    console.error("Error fetching archived streams:", error.message);
    throw new Error("Failed to fetch archived streams");
  }
};

// Получить метаданные стрима по stream_id
export const getStreamMetadata = async (streamId) => {
  try {
    const response = await fetch(`${API_BASE_URL}/metadata?stream_id=${streamId}`);
    return handleResponse(response);
  } catch (error) {
    console.error("Error fetching stream metadata:", error.message);
    throw new Error(`Failed to fetch metadata for stream ${streamId}`);
  }
};

// Получить метаданные архивного стрима по stream_name
export const getArchiveMetadata = async (streamName) => {
  try {
    const response = await fetch(`${API_BASE_URL}/archive/metadata?stream_name=${streamName}`);
    return handleResponse(response);
  } catch (error) {
    console.error("Error fetching archive metadata:", error.message);
    throw new Error(`Failed to fetch archive metadata for stream ${streamName}`);
  }
};
// Получить текущую конфигурацию сервера
export const getConfig = async () => {
  try {
    const response = await fetch(`${API_BASE_URL}/get-config`);
    return handleResponse(response);
  } catch (error) {
    console.error("Error fetching config:", error.message);
    throw new Error("Failed to fetch server configuration");
  }
};
// Запустить новый стрим
export const startStream = async (rtspUrl, streamId) => {
  try {
    const response = await fetch(`${API_BASE_URL}/start-stream`, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body: new URLSearchParams({
        rtsp_url: rtspUrl,
        stream_id: streamId,
      }),
    });
    return handleResponse(response);
  } catch (error) {
    console.error("Error starting stream:", error.message);
    throw new Error("Failed to start stream");
  }
};

// Остановить стрим
export const stopStream = async (streamId) => {
  try {
    const response = await fetch(`${API_BASE_URL}/stop-stream`, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body: new URLSearchParams({
        stream_id: streamId,
      }),
    });
    return handleResponse(response);
  } catch (error) {
    console.error("Error stopping stream:", error.message);
    throw new Error("Failed to stop stream");
  }
};

// Получить URL для HLS-потока
export const getStreamUrl = (streamName) => {
  return `${API_BASE_URL}/stream/${streamName}`;
};

// Получить URL для архивного потока
export const getArchiveUrl = (streamName) => {
  return `${API_BASE_URL}/archive/${streamName}`;
};

// Получить URL для превью
export const getPreviewUrl = (streamName) => {
  return `${API_BASE_URL}/preview/${streamName}`;
};

// Обновить конфигурацию сервера
export const updateConfig = async (config) => {
  try {
    const response = await fetch(`${API_BASE_URL}/update-config`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(config),
    });
    return handleResponse(response);
  } catch (error) {
    console.error("Error updating config:", error.message);
    throw new Error("Failed to update server configuration");
  }
};