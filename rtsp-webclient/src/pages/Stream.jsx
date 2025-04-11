import React from "react";
import { useParams } from "react-router-dom";
import StreamPlayer from "../components/StreamPlayer";
import { getStreamUrl } from "../utils/api";

const Stream = () => {
  const { streamName } = useParams();
  const streamUrl = getStreamUrl(streamName);

  return (
    <div className="mt-6">
      <h1 className="text-2xl font-semibold text-gray-800 mb-4">Stream: {streamName}</h1>
      <StreamPlayer streamUrl={streamUrl} />
    </div>
  );
};

export default Stream;