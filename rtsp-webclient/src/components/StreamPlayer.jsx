import React, { useEffect, useRef } from "react";
import { initializeHlsPlayer } from "./HlsPlayer";

const StreamPlayer = ({ streamUrl }) => {
  const videoRef = useRef(null);

  useEffect(() => {
    const cleanup = initializeHlsPlayer(videoRef, streamUrl);
    return cleanup;
  }, [streamUrl]);

  return (
    <div className="w-full max-w-4xl mx-auto">
      <video
        ref={videoRef}
        controls
        className="w-full rounded-lg shadow-lg"
        onError={(e) => console.error("Video error:", e)}
      />
    </div>
  );
};

export default StreamPlayer;