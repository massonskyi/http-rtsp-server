import React, { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import { getStreams, stopStream } from "../utils/api";
import PreviewModal from "./PreviewModal";

const StreamList = () => {
  const [streams, setStreams] = useState({});
  const [error, setError] = useState(null);
  const [selectedStream, setSelectedStream] = useState(null);

  const fetchStreams = async () => {
    try {
      const data = await getStreams();
      setStreams(data);
      setError(null);
    } catch (err) {
      setError("Failed to load streams");
    }
  };

  useEffect(() => {
    fetchStreams();
    const interval = setInterval(fetchStreams, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleStopStream = async (streamId, e) => {
    e.stopPropagation(); // Prevent navigation when clicking stop button
    try {
      await stopStream(streamId);
      fetchStreams();
    } catch (err) {
      setError("Failed to stop stream");
    }
  };

  const openPreview = (stream) => {
    setSelectedStream(stream);
  };

  const closePreview = () => {
    setSelectedStream(null);
  };

  return (
    <div className="mt-6">
      <h2 className="text-2xl font-semibold text-gray-800 mb-6">Active Streams</h2>
      {error && <p className="text-red-500 mb-4">{error}</p>}
      {Object.keys(streams).length === 0 ? (
        <p className="text-gray-600">No active streams</p>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
          {Object.entries(streams).map(([id, stream]) => (
            <Link 
              key={id} 
              to={`/stream/${stream.stream_name}`}
              className="block bg-white rounded-xl shadow-lg overflow-hidden transition-transform duration-200 hover:shadow-xl hover:scale-105 focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              <div className="relative aspect-video bg-gray-100">
                {stream.preview_url ? (
                  <img 
                    src={stream.preview_url} 
                    alt={`Preview of ${stream.stream_name}`}
                    className="w-full h-full object-cover"
                  />
                ) : (
                  <div className="flex items-center justify-center w-full h-full bg-gray-200">
                    <span className="text-gray-500">No preview</span>
                  </div>
                )}
                <div className="absolute top-2 right-2 px-2 py-1 bg-black bg-opacity-50 rounded text-xs text-white">
                  {stream.status}
                </div>
              </div>
              <div className="p-4">
                <div className="flex items-center justify-between mb-2">
                  <h3 className="font-medium text-gray-900 truncate">{stream.stream_name}</h3>
                </div>
                <div className="flex justify-between items-center">
                  <button
                    onClick={(e) => handleStopStream(stream.stream_name, e)}
                    className="px-3 py-1 bg-red-500 text-white text-sm rounded hover:bg-red-600 transition-colors"
                    aria-label="Stop stream"
                  >
                    Stop
                  </button>
                  {stream.preview_url && (
                    <button
                      onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        openPreview(stream);
                      }}
                      className="px-3 py-1 bg-blue-500 text-white text-sm rounded hover:bg-blue-600 transition-colors"
                    >
                      Quick View
                    </button>
                  )}
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}
      {selectedStream && (
        <PreviewModal stream={selectedStream} onClose={closePreview} />
      )}
    </div>
  );
};

export default StreamList;