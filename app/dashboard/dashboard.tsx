import React, { useEffect, useState, useRef } from 'react';
import { DashboardPartitionState } from './dashboard_pb';
import Chart from 'chart.js/auto';

const Dashboard: React.FC = () => {
  const [data, setData] = useState<{
    cumulativeTimesteps: number; 
    partitionIndex: number; 
    state: number[];
  }[]>([]);
  const chartRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const ws = new WebSocket('ws://localhost:2112/dashboard');

    ws.onopen = () => {
      console.log('Connected to WebSocket server');
    };

    ws.onmessage = (event: MessageEvent) => {
      // Parse the received protobuf message
      const decodedMessage = DashboardPartitionState.deserializeBinary(event.data);
      const cumulativeTimesteps = decodedMessage.getCumulativeTimesteps();
      const partitionIndex = decodedMessage.getPartitionIndex();
      const stateList = decodedMessage.getStateList();
      setData((prevData) => [
        ...prevData, {
          cumulativeTimesteps: cumulativeTimesteps, 
          partitionIndex: partitionIndex, 
          state: stateList
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

    const chartData = {
      labels: data.map((item) => item.partitionIndex),
      datasets: [
        {
          label: 'Trendline Plot',
          data: data.map((item) => item.cumulativeTimesteps),
          borderColor: 'rgba(75, 192, 192, 1)',
          borderWidth: 2,
          fill: false,
        },
      ],
    };

    const ctx = chartRef.current.getContext('2d');
    if (ctx) {
      new Chart(ctx, {
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