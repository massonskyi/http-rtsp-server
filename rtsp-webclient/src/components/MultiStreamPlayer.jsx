export default function MultiStreamPlayer({ streamIds }) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {streamIds.map((streamId) => (
          <div key={streamId} className="p-4 bg-gray-100 rounded shadow">
            <h2 className="text-lg font-bold">Стрим {streamId}</h2>
            <video
              controls
              autoPlay
              className="w-full"
              src={`/stream/${streamId}/playlist.m3u8`}
            />
          </div>
        ))}
      </div>
    );
  }