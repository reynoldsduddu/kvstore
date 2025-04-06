async function getCurrentLeader() {
  try {
    const response = await fetch("/api/leader");
    const data = await response.json();
    return data.leader;
  } catch (error) {
    console.error("Error fetching leader:", error);
    return null;
  }
}

async function updateLeaderStatus() {
  const leader = await getCurrentLeader();
  if (leader) {
    document.getElementById(
      "leader-status"
    ).textContent = `🔗 Connected to Leader: ${leader}`;
  } else {
    document.getElementById("leader-status").textContent =
      "⚠️ Could not determine leader.";
  }
}
//Auto Refresh Leader Status
setInterval(updateLeaderStatus, 3000);

document.getElementById("put-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const key = document.getElementById("put-key").value;
  const value = document.getElementById("put-value").value;

  const response = await fetch("/api/put", {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ key, value }),
  });

  if (response.ok) {
    alert("Key-value pair stored successfully!");
  } else {
    alert("Failed to store key-value pair.");
  }
});

document.getElementById("get-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const key = document.getElementById("get-key").value;

  const response = await fetch(`/api/get?key=${key}`);
  const data = await response.json();

  if (response.ok) {
    document.getElementById("get-result").textContent = `Value: ${data.value}`;
  } else {
    document.getElementById("get-result").textContent = "Key not found.";
  }
});

document.getElementById("delete-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const key = document.getElementById("delete-key").value;

  const response = await fetch(`/api/delete?key=${key}`, {
    method: "DELETE",
  });

  if (response.ok) {
    alert("Key-value pair deleted successfully!");
  } else {
    alert("Failed to delete key-value pair.");
  }
});

let currentPage = 1;
const limit = 10; // Number of items per page

// Fetch and display key-value pairs
async function fetchKeyValuePairs(page) {
  try {
    const response = await fetch(`/api/get-all?page=${page}&limit=${limit}`);

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const data = await response.json();

    const list = document.getElementById("key-value-list");
    list.innerHTML = ""; // Clear the list

    for (const [key, value] of Object.entries(data.data)) {
      const item = document.createElement("li");
      item.textContent = `${key}: ${value}`;
      list.appendChild(item);
    }

    // Update pagination
    document.getElementById(
      "page-info"
    ).textContent = `Page ${data.page} of ${data.totalPages}`;
    currentPage = data.page;

    document.getElementById("prev-page-button").disabled = currentPage === 1;
    document.getElementById("next-page-button").disabled =
      currentPage === data.totalPages;
  } catch (error) {
    console.error("Error fetching key-value pairs:", error);
  }
}

// Event listeners
document.getElementById("get-all-button").addEventListener("click", () => {
  fetchKeyValuePairs(currentPage);
});
document.getElementById("prev-page-button").addEventListener("click", () => {
  if (currentPage > 1) fetchKeyValuePairs(currentPage - 1);
});
document.getElementById("next-page-button").addEventListener("click", () => {
  fetchKeyValuePairs(currentPage + 1);
});

//Visualization for Weights and node liveness
setInterval(fetchAndRenderWeights, 4000);

let chart;

async function fetchAndRenderWeights() {
  try {
    const localLeaderRes = await fetch("/api/leader");
    const { leader } = await localLeaderRes.json();

    const [weightsRes, statusRes] = await Promise.all([
      fetch("/api/weights"),
      fetch("/api/status"),
    ]);

    if (!weightsRes || !statusRes) {
      // show fallback in UI
      document.querySelector("#statusTable tbody").innerHTML = `
        <tr><td colspan="2">❌ Leader unreachable. Retrying...</td></tr>
      `;
      return;
    }

    const weights = await weightsRes.json();
    const nodeStatus = await statusRes.json();

    const rawLabels = Object.keys(weights);
    const rawData = Object.values(weights);

    // Filter out nodes that are not alive
    const filtered = Object.entries(nodeStatus).map(([fullAddr, isAlive]) => {
      const weight = weights[fullAddr] ?? 0; // default to 0 if missing
      return {
        label: fullAddr,
        val: weight,
        isAlive: isAlive,
        fullKey: fullAddr,
      };
    });

    const labels = filtered.map((d) => d.label);
    const data = filtered.map((d) => (d.val === 0 ? 0.05 : d.val));
    const sorted = [...data].sort((a, b) => b - a);

    // Color based on rank
    const getColor = (val, isAlive) => {
      if (!isAlive) return "red"; // 🔴 dead node
      if (val === 0) return "lightgray"; // 🩶 alive but didn't participate
      const rank = sorted.indexOf(val); // 🟡 normal ranking
      if (rank === 0) return "green";
      if (rank === sorted.length - 1) return "gold";
      return "gold";
    };

    const backgroundColors = filtered.map((d) => getColor(d.val, d.isAlive));
    const borderColors = [...backgroundColors];

    const labeled = labels.map((label) =>
      label === leader ? `${label} ⭐` : label
    );

    // Chart
    if (!chart) {
      const ctx = document.getElementById("weightChart").getContext("2d");
      chart = new Chart(ctx, {
        type: "bar",
        data: {
          labels: labeled,
          datasets: [
            {
              label: "Cabinet Weights + Health",
              data: data.map((val) => (val === 0 ? 0.05 : val)), // hack to show 0 bars
              backgroundColor: backgroundColors,
              borderColor: borderColors,
              borderWidth: 1,
            },
          ],
        },
        options: {
          animation: false,
          responsive: true,
          plugins: {
            tooltip: {
              callbacks: {
                label: function (context) {
                  const index = context.dataIndex;
                  const label = context.chart.data.labels[index];
                  const weight = context.dataset.data[index];
                  const full = filtered[index];

                  if (!full.isAlive) return `${label}: ❌ DEAD NODE`;

                  const aliveWeights = filtered
                    .filter((d) => d.isAlive)
                    .map((d) => d.val);

                  const allEqual =
                    aliveWeights.length > 0 &&
                    aliveWeights.every(
                      (w) => Math.abs(w - aliveWeights[0]) < 0.001
                    );

                  if (allEqual) return `${label}: ✅ (Equal Weights Assigned)`;

                  if (full.val === 0)
                    return `${label}: 💤 Alive (No Participation This Round)`;

                  return `${label}: ✅ Weight ${full.val.toFixed(2)}`;
                },
              },
            },
            legend: {
              display: false,
            },
          },
          scales: {
            y: {
              beginAtZero: true,
            },
          },
        },
      });
    } else {
      chart.data.labels = labeled;
      chart.data.datasets[0].data = data.map((val) => (val === 0 ? 0.05 : val));
      chart.data.datasets[0].backgroundColor = backgroundColors;
      chart.data.datasets[0].borderColor = borderColors;
      chart.update();
    }

    // Node Health Table
    const statusTable = document.querySelector("#statusTable tbody");
    statusTable.innerHTML = "";
    Object.entries(nodeStatus).forEach(([node, isAlive]) => {
      const row = document.createElement("tr");

      const nameCell = document.createElement("td");
      nameCell.textContent = node;

      const statusCell = document.createElement("td");
      statusCell.textContent = isAlive ? "🟢 Alive" : "🔴 Dead";
      statusCell.style.color = isAlive ? "green" : "red";

      row.appendChild(nameCell);
      row.appendChild(statusCell);
      statusTable.appendChild(row);
    });
  } catch (err) {
    console.error("⚠️ Failed to fetch chart/status from leader:", err);
  }
}

window.onload = () => {
  updateLeaderStatus();
  fetchKeyValuePairs(currentPage);
  fetchAndRenderWeights();
};
