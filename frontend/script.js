document.getElementById("put-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const key = document.getElementById("put-key").value;
  const value = document.getElementById("put-value").value;

  const response = await fetch("http://localhost:8081/put", {
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

  const response = await fetch(`http://localhost:8081/get?key=${key}`);
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

  const response = await fetch(`http://localhost:8081/delete?key=${key}`, {
    method: "DELETE",
  });

  if (response.ok) {
    alert("Key-value pair deleted successfully!");
  } else {
    alert("Failed to delete key-value pair.");
  }
});

document
  .getElementById("get-all-button")
  .addEventListener("click", async () => {
    const response = await fetch("http://localhost:8081/get-all");
    const data = await response.json();

    const list = document.getElementById("key-value-list");
    list.innerHTML = ""; // Clear the list

    for (const [key, value] of Object.entries(data)) {
      const item = document.createElement("li");
      item.textContent = `${key}: ${value}`;
      list.appendChild(item);
    }
  });

let currentPage = 1;
const limit = 10; // Number of items per page

// Function to fetch and display key-value pairs
async function fetchKeyValuePairs(page) {
  try {
    const response = await fetch(
      `http://localhost:8081/get-all?page=${page}&limit=${limit}`
    );
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

    // Update pagination controls
    document.getElementById(
      "page-info"
    ).textContent = `Page ${data.page} of ${data.totalPages}`;
    currentPage = data.page;

    // Enable/disable Previous and Next buttons
    document.getElementById("prev-page-button").disabled = currentPage === 1;
    document.getElementById("next-page-button").disabled =
      currentPage === data.totalPages;
  } catch (error) {
    console.error("Error fetching key-value pairs:", error);
  }
}

// Event listener for the Refresh List button
document.getElementById("get-all-button").addEventListener("click", () => {
  fetchKeyValuePairs(currentPage);
});

// Event listener for the Previous button
document.getElementById("prev-page-button").addEventListener("click", () => {
  if (currentPage > 1) {
    fetchKeyValuePairs(currentPage - 1);
  }
});

// Event listener for the Next button
document.getElementById("next-page-button").addEventListener("click", () => {
  fetchKeyValuePairs(currentPage + 1);
});

// Fetch the first page on page load
fetchKeyValuePairs(currentPage);
