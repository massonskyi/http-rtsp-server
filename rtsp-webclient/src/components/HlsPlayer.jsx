import Hls from "hls.js";

export const initializeHlsPlayer = (videoRef, streamUrl) => {
  if (!videoRef.current || !streamUrl) return;

  const video = videoRef.current;

  // Проверяем, поддерживает ли браузер HLS через hls.js
  if (Hls.isSupported()) {
    const hls = new Hls();
    hls.loadSource(streamUrl);
    hls.attachMedia(video);
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      video.play().catch((error) => {
        console.error("Error playing video:", error.message);
      });
    });
    hls.on(Hls.Events.ERROR, (event, data) => {
      if (data.fatal) {
        console.error("Fatal HLS error:", data);
        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            hls.startLoad();
            break;
          case Hls.ErrorTypes.MEDIA_ERROR:
            hls.recoverMediaError();
            break;
          default:
            hls.destroy();
            break;
        }
      }
    });
    return () => {
      hls.destroy();
    };
  } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
    // Если браузер поддерживает HLS нативно (например, Safari)
    video.src = streamUrl;
    video.addEventListener("loadedmetadata", () => {
      video.play().catch((error) => {
        console.error("Error playing video natively:", error.message);
      });
    });
  } else {
    console.error("HLS is not supported in this browser");
  }
};