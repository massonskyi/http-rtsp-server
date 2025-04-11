import React, { useState } from "react";
import StreamList from "../components/StreamList";
import { startStream } from "../utils/api";

const Home = () => {
  const [rtspUrl, setRtspUrl] = useState("");
  const [streamId, setStreamId] = useState("");
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);

  const handleStartStream = async (e) => {
    e.preventDefault();
    try {
      await startStream(rtspUrl, streamId);
      setSuccess(`Stream ${streamId} started successfully`);
      setError(null);
      setRtspUrl("");
      setStreamId("");
    } catch (err) {
      setError("Failed to start stream");
      setSuccess(null);
    }
  };

  return (
    <div className="space-y-6">
      <h1 className="text-3xl font-bold text-gray-800">RTSP Stream Manager</h1>
      <form onSubmit={handleStartStream} className="space-y-4">
        <div className="flex items-center space-x-4">
          <label className="w-24 text-gray-700">RTSP URL:</label>
          <input
            type="text"
            value={rtspUrl}
            onChange={(e) => setRtspUrl(e.target.value)}
            placeholder="rtsp://..."
            required
            className="flex-1 p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
        <div className="flex items-center space-x-4">
          <label className="w-24 text-gray-700">Stream ID:</label>
          <input
            type="text"
            value={streamId}
            onChange={(e) => setStreamId(e.target.value)}
            placeholder="stream1"
            required
            className="flex-1 p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
        <button
          type="submit"
          className="w-full px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600"
        >
          Start Stream
        </button>
      </form>
      {error && <p className="text-red-500">{error}</p>}
      {success && <p className="text-green-500">{success}</p>}
      <StreamList />
    </div>
  );
};

export default Home;