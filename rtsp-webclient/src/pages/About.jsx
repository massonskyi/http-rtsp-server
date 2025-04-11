import React, { useState, useEffect } from "react";
import { getConfig, updateConfig } from "../utils/api";

const About = () => {
  const [config, setConfig] = useState(null);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);
  const [isEditing, setIsEditing] = useState(false);
  const [formData, setFormData] = useState({
    HLSOutputDir: "",
    RTSPPort: "",
    HTTPPort: "",
  });

  // Загружаем конфигурацию при монтировании компонента
  useEffect(() => {
    const loadConfig = async () => {
      try {
        const data = await getConfig();
        setConfig(data);
        setFormData({
          HLSOutputDir: data.hls_output_dir,
          RTSPPort: data.rtsp_port,
          HTTPPort: data.http_port,
        });
        setError(null);
      } catch (err) {
        setError(err.message);
      }
    };
    loadConfig();
  }, []);

  // Обработчик изменения полей формы
  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setFormData((prev) => ({
      ...prev,
      [name]: value,
    }));
  };

  // Обработчик отправки формы
  const handleSubmit = async (e) => {
    e.preventDefault();
    try {
      await updateConfig({
        hls_output_dir: formData.HLSOutputDir,
        rtsp_port: formData.RTSPPort,
        http_port: formData.HTTPPort,
      });
      setSuccess("Configuration updated successfully");
      setError(null);
      setIsEditing(false);
      // Обновляем отображаемую конфигурацию
      setConfig({
        ...config,
        hls_output_dir: formData.HLSOutputDir,
        rtsp_port: formData.RTSPPort,
        http_port: formData.HTTPPort,
      });
    } catch (err) {
      setError(err.message);
      setSuccess(null);
    }
  };

  return (
    <div className="mt-6 space-y-6">
      <h1 className="text-3xl font-bold text-gray-800">About RTSP Stream Manager</h1>
      <p className="text-gray-600">
        This project is designed to manage RTSP streams, allowing users to start, stop, and archive streams with ease.
        It provides a web interface to interact with the server and view live or archived streams.
      </p>

      <h2 className="text-2xl font-semibold text-gray-800">Server Configuration</h2>
      {error && <p className="text-red-500">{error}</p>}
      {success && <p className="text-green-500">{success}</p>}

      {config ? (
        <div>
          {isEditing ? (
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="flex items-center space-x-4">
                <label className="w-32 text-gray-700">HLS Output Dir:</label>
                <input
                  type="text"
                  name="HLSOutputDir"
                  value={formData.HLSOutputDir}
                  onChange={handleInputChange}
                  className="flex-1 p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                  required
                />
              </div>
              <div className="flex items-center space-x-4">
                <label className="w-32 text-gray-700">RTSP Port:</label>
                <input
                  type="text"
                  name="RTSPPort"
                  value={formData.RTSPPort}
                  onChange={handleInputChange}
                  className="flex-1 p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                  required
                />
              </div>
              <div className="flex items-center space-x-4">
                <label className="w-32 text-gray-700">HTTP Port:</label>
                <input
                  type="text"
                  name="HTTPPort"
                  value={formData.HTTPPort}
                  onChange={handleInputChange}
                  className="flex-1 p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                  required
                />
              </div>
              <div className="flex space-x-4">
                <button
                  type="submit"
                  className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600"
                >
                  Save
                </button>
                <button
                  type="button"
                  onClick={() => setIsEditing(false)}
                  className="px-4 py-2 bg-gray-500 text-white rounded-lg hover:bg-gray-600"
                >
                  Cancel
                </button>
              </div>
            </form>
          ) : (
            <div className="bg-white p-4 rounded-lg shadow">
              <p><strong>HLS Output Dir:</strong> {config.hls_output_dir}</p>
              <p><strong>RTSP Port:</strong> {config.rtsp_port}</p>
              <p><strong>HTTP Port:</strong> {config.http_port}</p>
              <button
                onClick={() => setIsEditing(true)}
                className="mt-4 px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600"
              >
                Edit Configuration
              </button>
            </div>
          )}
        </div>
      ) : (
        <p className="text-gray-600">Loading configuration...</p>
      )}
    </div>
  );
};

export default About;