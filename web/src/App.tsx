import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "@/components/app-shell";
import ServersPage from "@/pages/servers";
import ServerDetailPage from "@/pages/server-detail";
import TunnelsPage from "@/pages/tunnels";
import LogsPage from "@/pages/logs";
import SettingsPage from "@/pages/settings";

export default function App() {
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={<ServersPage />} />
        <Route path="/servers/:id" element={<ServerDetailPage />} />
        <Route path="/tunnels" element={<TunnelsPage />} />
        <Route path="/logs" element={<LogsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AppShell>
  );
}

