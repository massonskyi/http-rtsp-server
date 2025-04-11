import React from "react";
import { useParams } from "react-router-dom";
import StreamPlayer from "../components/StreamPlayer";
import { getArchiveUrl } from "../utils/api";

const Archive = () => {
  const { streamName } = useParams();
  const archiveUrl = getArchiveUrl(streamName);

  return (
    <div className="mt-6">
      <h1 className="text-2xl font-semibold text-gray-800 mb-4">Archive: {streamName}</h1>
      <StreamPlayer streamUrl={archiveUrl} />
    </div>
  );
};

export default Archive;