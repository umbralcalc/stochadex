import React, { useEffect, useState, useRef, useMemo } from 'react';
import { DashboardPartitionState } from './dashboard_state';
import Chart from 'chart.js/auto';
import zoomPlugin from 'chartjs-plugin-zoom';


const Dashboard: React.FC = () => {
  const [data, setData] = useState<{
    cumulativeTimesteps: number;
    partitionName: string;
    state: number[];
  }[]>([]);
  const [chartUpdatesEnabled, setChartUpdatesEnabled] = useState(true);
  const [selectedPartitionName, setSelectedPartitionName] = useState<string | null>(null);
  const chartRef = useRef<HTMLCanvasElement | null>(null);
  const chartInstanceRef = useRef<Chart | null>(null);
  const [lineColours, setLineColours] = useState<{
    [partitionName: string]: { [index: number]: string }
  }>({});
  Chart.register(zoomPlugin);

  function getRandomColour() {
    const letters = '0123456789ABCDEF';
    let color = '#';
    const minBrightness = 0.6;
  
    for (let i = 0; i < 6; i++) {
      color += letters[Math.floor(Math.random() * 16)];
    }
  
    // Get the RGB components of the color
    const red = parseInt(color.substring(1, 3), 16);
    const green = parseInt(color.substring(3, 5), 16);
    const blue = parseInt(color.substring(5, 7), 16);
  
    // Calculate the brightness of the color (normalized value between 0 and 1)
    const brightness = (0.299 * red + 0.587 * green + 0.114 * blue) / 255;
  
    // If the brightness is below the minimum, adjust the color to make it brighter
    if (brightness < minBrightness) {
      const correctionFactor = minBrightness / brightness;
      const newRed = Math.min(255, Math.floor(red * correctionFactor));
      const newGreen = Math.min(255, Math.floor(green * correctionFactor));
      const newBlue = Math.min(255, Math.floor(blue * correctionFactor));
  
      // Convert the RGB components back to hexadecimal and update the color
      color = `#${(newRed < 16 ? '0' : '') + newRed.toString(16)}` +
              `${(newGreen < 16 ? '0' : '') + newGreen.toString(16)}` +
              `${(newBlue < 16 ? '0' : '') + newBlue.toString(16)}`;
    }
  
    return color;
  }

  // create a memoized version of a function that generates the datasets
  const datasets = useMemo(() => {
    const result: { [partitionName: string]: {
      label: string;
      data: {
        x: number;
        y: number;
      }[];
      borderColor: string;
      borderWidth: number;
      fill: boolean;
    }[]} = {};

    data.forEach((datum) => {
      for (let index = 0; index < datum.state.length; index++) {
        const partitionName = datum.partitionName;

        if (!(partitionName in lineColours)) {
          setLineColours((prevLineColours) => ({
            ...prevLineColours,
            [partitionName]: {},
          }));
          lineColours[partitionName] = {}
        }

        if (!(index in lineColours[partitionName])) {
          const colour = getRandomColour()
          setLineColours((prevLineColours) => ({
            ...prevLineColours,
            [partitionName]: {
              ...prevLineColours[partitionName],
              [index]: colour,
            },
          }));
          lineColours[partitionName][index] = colour
        }

        if (!(partitionName in result)) {
          result[partitionName] = [];
        }

        if (!(index in result[partitionName])) {
          result[partitionName].push({
            label: `Element ${index}`,
            data: [{
              x: datum.cumulativeTimesteps,
              y: datum.state[index],
            }],
            borderColor: lineColours[partitionName][index],
            borderWidth: 2,
            fill: false,
          });
        } else {
          result[partitionName][index].data.push({
            x: datum.cumulativeTimesteps,
            y: datum.state[index],
          })
        }
      }
    });

    return result;
  }, [data, lineColours]);

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
          partitionName: decodedMessage.partition_name, 
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

  // Implement a function to update the selected partitionName
  const handlePartitionNameChange = (partitionName: string) => {
    setSelectedPartitionName(partitionName);
  };

  const resetZoom = () => {
    if (chartInstanceRef.current) {
      chartInstanceRef.current.resetZoom();
    }
  };

  const handleKeyPress = (event: KeyboardEvent) => {
    if (event.key === 'r') {
      resetZoom();
    }
  };

  useEffect(() => {
    window.addEventListener('keypress', handleKeyPress);

    // Clean up the event listener when the component unmounts
    return () => {
      window.removeEventListener('keypress', handleKeyPress);
    };
  }, []);

  useEffect(() => {
    if (!chartRef.current || selectedPartitionName === null) return;

    if (!datasets[selectedPartitionName].length) return;

    if (chartInstanceRef.current) {
      chartInstanceRef.current.destroy();
    }

    const chartData = {
      datasets: datasets[selectedPartitionName],
    };

    const ctx = chartRef.current.getContext('2d');
    if (ctx) {
      chartRef.current.height = 300;
      chartInstanceRef.current = new Chart(ctx, {
        type: 'line',
        data: chartData,
        options: {
          responsive: true,
          maintainAspectRatio: false,
          animation: {
            duration: 0
          },
          scales: {
            x: {
              type: 'linear',
              position: 'bottom',
              grid: {
                display: false
              },
              ticks: {
                color: 'white'
              },
            },
            y: {
              display: true,
              grid: {
                display: false
              },
              ticks: {
                color: 'white'
              },
            },
          },
          plugins: {
            legend: {
              labels: {
                color: 'white'
              }
            },
            title: {
              display: false
            },
            zoom: {
              pan: {
                enabled: true,
                mode: 'xy',
                modifierKey: 'ctrl',
              },
              zoom: {
                drag: {
                  enabled: true,
                  modifierKey: 'shift',
                },
                wheel: {
                  enabled: true,
                },
                pinch: {
                  enabled: true
                },
                mode: 'xy',
              },
            },
          },
          elements: {
            point: {
              borderColor: 'white',
              borderWidth: 1
            },
            line: {
              borderColor: 'white',
              borderWidth: 1
            }
          }
        },
      });
    }
  }, [selectedPartitionName, chartUpdatesEnabled && datasets]);
  
  const handleToggleChartUpdates = () => {
    setChartUpdatesEnabled((prev) => !prev); // Toggle the state between true and false
  };

  return (
    <div>
      <div className="flex items-center justify-center h-64 border border-gray-300 rounded-lg p-4">
        <canvas ref={chartRef} width="400" height="200" />
      </div>
      <div>
        <div>
          <button onClick={handleToggleChartUpdates}>
            {chartUpdatesEnabled ? 'Disable Live Updates' : 'Enable Live Updates'}
          </button><br/>
          {Object.entries(datasets).map(([k, v]) => (
            <button
              key={k}
              onClick={() => handlePartitionNameChange(k)}
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
