import React, { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import { getArchivedStreams } from "../utils/api";
import PreviewModal from "./PreviewModal";

const ArchiveList = () => {
  const [archives, setArchives] = useState({});
  const [error, setError] = useState(null);
  const [selectedStream, setSelectedStream] = useState(null);

  const fetchArchives = async () => {
    try {
      const data = await getArchivedStreams();
      setArchives(data);
      setError(null);
    } catch (err) {
      setError("Failed to load archived streams");
    }
  };

  useEffect(() => {
    fetchArchives();
  }, []);

  const openPreview = (stream) => {
    setSelectedStream(stream);
  };

  const closePreview = () => {
    setSelectedStream(null);
  };

  return (
    <div className="mt-6">
      <h2 className="text-2xl font-semibold text-gray-800 mb-4">Archived Streams</h2>
      {error && <p className="text-red-500 mb-4">{error}</p>}
      {Object.keys(archives).length === 0 ? (
        <p className="text-gray-600">No archived streams</p>
      ) : (
        <ul className="space-y-3">
          {Object.entries(archives).map(([id, archive]) => (
            <li key={id} className="flex items-center justify-between p-3 bg-white rounded-lg shadow">
              <Link to={`/archive/${archive.stream_name}`} className="text-blue-600 hover:underline">
                {archive.stream_name} (Duration: {archive.duration}s)
              </Link>
              {archive.preview_url && (
                <button
                  onClick={() => openPreview(archive)}
                  className="px-3 py-1 bg-blue-500 text-white rounded hover:bg-blue-600"
                >
                  Preview
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {selectedStream && (
        <PreviewModal stream={selectedStream} onClose={closePreview} />
      )}
    </div>
  );
};

export default ArchiveList;