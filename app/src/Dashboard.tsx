import React, { useEffect, useState, useRef } from 'react';
import { DashboardPartitionState } from './dashboard_state';
import Chart from 'chart.js/auto';

const Dashboard: React.FC = () => {
  const [data, setData] = useState<{
    cumulativeTimesteps: number; 
    partitionIndex: number; 
    state: number[];
  }[]>([]);
  const chartRef = useRef<HTMLCanvasElement | null>(null);
  const chartInstanceRef = useRef<Chart | null>(null);

  useEffect(() => {
    const ws = new WebSocket('ws://localhost:2112/dashboard');

    ws.onopen = () => {
      console.log('Connected to WebSocket server');
    };

    ws.onmessage = async (event: MessageEvent) => {
      const decodedMessage = DashboardPartitionState.deserializeBinary(
        Uint8Array.from(event.data)
      );
      setData((prevData) => [
        ...prevData, {
          cumulativeTimesteps: decodedMessage.cumulative_timesteps, 
          partitionIndex: decodedMessage.partition_index, 
          state: decodedMessage.state
        },
      ]);
    };

    ws.onclose = () => {
      console.log('Disconnected from WebSocket server');
    };

    return () => {
      ws.close();
    };
  }, []);

  useEffect(() => {
    if (!chartRef.current || !data.length) return;

    // Destroy the previous Chart instance if it exists
    if (chartInstanceRef.current) {
      chartInstanceRef.current.destroy();
    }

    const chartData = {
      labels: data.map((item) => item.partitionIndex),
      datasets: [
        {
          label: 'Trendline Plot',
          data: data.map((item) => ({
            x: item.cumulativeTimesteps,
            y: item.state[item.partitionIndex],
          })),
          borderColor: 'rgba(75, 192, 192, 1)',
          borderWidth: 2,
          fill: false,
        },
      ],
    };

    const ctx = chartRef.current.getContext('2d');
    if (ctx) {
      chartInstanceRef.current = new Chart(ctx, {
        type: 'line',
        data: chartData,
        options: {
          responsive: true,
          maintainAspectRatio: false,
          scales: {
            x: {
              display: true,
            },
            y: {
              display: true,
            },
          },
        },
      });
    }
  }, [data]);

  return (
    <div className="flex items-center justify-center h-64 border border-gray-300 rounded-lg p-4">
      <canvas ref={chartRef} width="400" height="200" />
    </div>
  );
};

export default Dashboard;