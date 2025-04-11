import React from "react";
import { Link } from "react-router-dom";

const Header = () => {
  return (
    <header className="bg-blue-600 text-white p-4 shadow">
      <nav className="container mx-auto flex justify-between items-center">
        <Link to="/" className="text-xl font-bold">
          RTSP Stream Manager
        </Link>
        <div className="space-x-4">
          <Link to="/" className="hover:underline">
            Home
          </Link>
          <Link to="/archive" className="hover:underline">
            Archive
          </Link>
          <Link to="/about" className="hover:underline">
            About
          </Link>
        </div>
      </nav>
    </header>
  );
};

export default Header;