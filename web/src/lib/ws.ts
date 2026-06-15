import { useEffect, useState } from "react";
import { EventsOn, EventsOff } from "../../wailsjs/runtime";
import * as App from "../../wailsjs/go/main/App";

export type WsStatus = "connecting" | "open" | "closed";

// useWebSocket maps a path to Wails Go-to-JS events, simulating a WebSocket connection.
export function useWebSocket(path: string | null, onMessage: (data: string) => void): WsStatus {
  const [status, setStatus] = useState<WsStatus>("closed");

  useEffect(() => {
    if (!path) {
      setStatus("closed");
      return;
    }

    setStatus("connecting");

    let eventName = "";
    let serverId = "";
    let isLogStream = false;
    let isPlayitLog = false;

    if (path.startsWith("/ws/logs/")) {
      serverId = path.substring("/ws/logs/".length);
      eventName = "logs:" + serverId;
      isLogStream = true;
    } else if (path.startsWith("/ws/playit-logs/")) {
      serverId = path.substring("/ws/playit-logs/".length);
      eventName = "logs:playit:" + serverId;
      isPlayitLog = true;
    } else if (path.startsWith("/ws/status/")) {
      serverId = path.substring("/ws/status/".length);
      eventName = "topic:" + serverId;
    } else if (path.startsWith("/ws/playit/")) {
      serverId = path.substring("/ws/playit/".length);
      eventName = "topic:playit:" + serverId;
    }

    if (!eventName) {
      setStatus("closed");
      return;
    }

    // Subscribe to Wails event
    EventsOn(eventName, (data: any) => {
      console.log("[useWebSocket] Received event:", eventName, data);
      setStatus("open");
      if (typeof data === "string") {
        onMessage(data);
      } else {
        onMessage(JSON.stringify(data));
      }
    });

    // Start background stream if needed
    if (isLogStream) {
      App.StartStreamingLogs(serverId);
    } else if (isPlayitLog) {
      App.StartStreamingPlayitLogs(serverId);
    }

    if (!isLogStream && !isPlayitLog) {
      setStatus("open");
    }

    return () => {
      EventsOff(eventName);
      if (isLogStream) {
        App.StopStreamingLogs(serverId);
      } else if (isPlayitLog) {
        App.StopStreamingPlayitLogs(serverId);
      }
      setStatus("closed");
    };
  }, [path]);

  return status;
}
