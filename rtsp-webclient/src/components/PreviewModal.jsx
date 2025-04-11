import React from "react";
import { getPreviewUrl } from "../utils/api";

const PreviewModal = ({ stream, onClose }) => {
  const previewUrl = getPreviewUrl(stream.stream_name);

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white p-6 rounded-lg shadow-lg max-w-md w-full">
        <h3 className="text-lg font-semibold text-gray-800 mb-4">
          Preview for {stream.stream_name}
        </h3>
        <img src={previewUrl} alt="Stream Preview" className="w-full rounded-lg mb-4" />
        <button
          onClick={onClose}
          className="w-full px-4 py-2 bg-gray-500 text-white rounded hover:bg-gray-600"
        >
          Close
        </button>
      </div>
    </div>
  );
};

export default PreviewModal;