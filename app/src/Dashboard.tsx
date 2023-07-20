import React, { useEffect, useState, useRef, useMemo } from 'react';
import { DashboardPartitionState } from './dashboard_state';
import Chart from 'chart.js/auto';

const Dashboard: React.FC = () => {
  const [data, setData] = useState<{
    cumulativeTimesteps: number;
    partitionIndex: number;
    state: number[];
  }[]>([]);
  const [selectedPartitionIndex, setSelectedPartitionIndex] = useState<string | null>(null);
  const chartRef = useRef<HTMLCanvasElement | null>(null);
  const chartInstanceRef = useRef<Chart | null>(null);

  // create a memoized version of a function that generates the datasets
  const datasets = useMemo(() => {
    const result: { [partitionIndex: string]: {
      label: string;
      data: {
        x: number;
        y: number;
      }[];
      borderColor: string;
      borderWidth: number;
      fill: boolean;
    }[]} = {};

    function getRandomColor() {
      const letters = '0123456789ABCDEF';
      let color = '#';
      for (let i = 0; i < 6; i++) {
        color += letters[Math.floor(Math.random() * 16)];
      }
      return color;
    }

    data.forEach((datum) => {
      for (let index = 0; index < datum.state.length; index++) {
        const partitionIndexString = String(datum.partitionIndex);

        if (!(partitionIndexString in result)) {
          result[partitionIndexString] = [];
        }

        if (!(index in result[partitionIndexString])) {
          result[partitionIndexString].push({
            label: `Element ${index}`,
            data: [{
              x: datum.cumulativeTimesteps,
              y: datum.state[index],
            }],
            borderColor: getRandomColor(),
            borderWidth: 2,
            fill: false,
          });
        } else {
          result[partitionIndexString][index].data.push({
            x: datum.cumulativeTimesteps,
            y: datum.state[index],
          })
        }
      }
    });

    return result;
  }, [data]);

  useEffect(() => {
    const ws = new WebSocket('ws://localhost:2112/dashboard');
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      console.log('Connected to WebSocket server');
    };

    ws.onmessage = async (event: MessageEvent) => {
      const decodedMessage = DashboardPartitionState.deserializeBinary(event.data);
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

  // Implement a function to update the selected partitionIndex
  const handlePartitionIndexChange = (partitionIndex: string) => {
    setSelectedPartitionIndex(partitionIndex);
  };

  useEffect(() => {
    if (!chartRef.current || !data.length || selectedPartitionIndex === null) return;

    // Destroy the previous Chart instance if it exists
    if (chartInstanceRef.current) {
      chartInstanceRef.current.destroy();
    }

    const chartData = {
      datasets: datasets[selectedPartitionIndex],
    };

    const ctx = chartRef.current.getContext('2d');
    if (ctx) {
      chartInstanceRef.current = new Chart(ctx, {
        type: 'scatter',
        data: chartData,
        options: {
          responsive: true,
          maintainAspectRatio: false,
          scales: {
            x: {
              type: 'linear',
              position: 'bottom',
            },
            y: {
              display: true,
            },
          },
        },
      });
    }
  }, [data, selectedPartitionIndex, datasets]);

  return (
    <div>
      <div className="flex items-center justify-center h-64 border border-gray-300 rounded-lg p-4">
        <canvas ref={chartRef} width="400" height="200" />
      </div>
      <div>
        <div>
          {Object.entries(datasets).map(([k, v]) => (
            <button
              key={k}
              onClick={() => handlePartitionIndexChange(k)}
            >
              Show Partition {k}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
};

export default Dashboard;
