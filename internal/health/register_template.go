package health

import "html/template"

var registerPageTemplate = template.Must(template.New("register-page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CMON — Register Local Complaint</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700&family=DM+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    :root {
      /* Surfaces — refined warm-paper */
      --bg: #f1ede4;
      --surface: #ffffff;
      --surface-raised: #fbf8f2;
      --surface-bright: #ece6d8;
      --surface-deep: #e3ddcd;

      /* Borders */
      --border: rgba(40,32,20,0.09);
      --border-bright: rgba(40,32,20,0.16);
      --border-strong: rgba(40,32,20,0.28);

      /* Text */
      --text: #14181f;
      --text-2: #2f3742;
      --text-dim: #5b6472;
      --text-faint: #8b95a5;

      /* Accents */
      --accent: #1f5fe8;
      --accent-hover: #1648c9;
      --accent-dim: rgba(31,95,232,0.10);
      --accent-soft: rgba(31,95,232,0.05);

      --danger: #dc2626;
      --danger-dim: rgba(220,38,38,0.09);
      --success: #15803d;
      --success-dim: rgba(21,128,61,0.10);
      --warn: #b45309;

      /* Shadows */
      --shadow-sm: 0 1px 2px rgba(20,16,8,0.04);
      --shadow-md: 0 1px 3px rgba(20,16,8,0.06), 0 1px 2px rgba(20,16,8,0.03);
      --shadow-lg: 0 6px 24px rgba(20,16,8,0.07), 0 1px 3px rgba(20,16,8,0.04);

      /* Type */
      --font-mono: 'JetBrains Mono', 'SF Mono', 'Roboto Mono', monospace;
      --font-sans: 'DM Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;

      /* Radii */
      --r-xs: 4px;
      --r-sm: 6px;
      --r-md: 8px;
      --r-lg: 12px;
    }

    *,*::before,*::after { box-sizing: border-box; }
    html { scroll-behavior: smooth; -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; }

    body {
      margin: 0;
      min-height: 100vh;
      background: var(--bg);
      color: var(--text);
      font-family: var(--font-sans);
      font-size: 14px;
      line-height: 1.5;
      display: flex;
      flex-direction: column;
      justify-content: center;
      align-items: center;
      padding: 40px 20px;
    }

    /* ── Subtle paper texture ── */
    body::before {
      content: "";
      position: fixed;
      inset: 0;
      z-index: 9999;
      pointer-events: none;
      opacity: 0.035;
      background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.75' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
      background-size: 200px;
    }

    .form-container {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-lg);
      padding: 36px;
      width: 100%;
      max-width: 600px;
      box-shadow: var(--shadow-lg);
      animation: modal-in 0.25s ease-out;
    }

    @keyframes modal-in {
      from { opacity: 0; transform: scale(0.97) translateY(12px); }
      to { opacity: 1; transform: scale(1) translateY(0); }
    }

    .header {
      margin-bottom: 28px;
      border-bottom: 1px solid var(--border);
      padding-bottom: 16px;
    }

    .title {
      font-size: 22px;
      font-weight: 700;
      letter-spacing: -0.02em;
      color: var(--text);
      margin: 0 0 6px 0;
      display: flex;
      align-items: center;
      gap: 10px;
    }

    .subtitle {
      font-size: 13px;
      color: var(--text-dim);
      margin: 0;
    }

    .form-row {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 20px;
      margin-bottom: 20px;
    }

    @media (max-width: 580px) {
      .form-row {
        grid-template-columns: 1fr;
        gap: 16px;
        margin-bottom: 16px;
      }
    }

    .form-group {
      margin-bottom: 20px;
    }

    .form-group.full-width {
      grid-column: span 2;
    }

    @media (max-width: 580px) {
      .form-group.full-width {
        grid-column: span 1;
      }
    }

    .label {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--text-dim);
      margin-bottom: 6px;
      display: flex;
      align-items: center;
    }

    .required-star {
      color: var(--danger);
      margin-left: 3px;
      font-size: 12px;
    }

    .input-field {
      width: 100%;
      height: 40px;
      padding: 8px 12px;
      border: 1px solid var(--border);
      border-radius: var(--r-sm);
      background: var(--surface-raised);
      font-family: var(--font-sans);
      font-size: 13px;
      color: var(--text);
      outline: none;
      transition: border-color 0.15s, box-shadow 0.15s;
    }

    .input-field:focus {
      border-color: var(--accent);
      box-shadow: 0 0 0 3px var(--accent-dim);
      background: var(--surface);
    }

    select.input-field {
      cursor: pointer;
      appearance: none;
      background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 24 24' fill='none' stroke='rgba(40,32,20,0.5)' stroke-width='2' stroke-linecap='round' stroke-linejoin='round' xmlns='http://www.w3.org/2000/svg'%3E%3Cpolyline points='6 9 12 15 18 9'/%3E%3C/svg%3E");
      background-repeat: no-repeat;
      background-position: right 12px center;
      background-size: 16px;
      padding-right: 36px;
    }

    textarea.input-field {
      height: auto;
      resize: vertical;
      min-height: 80px;
    }

    .actions {
      display: flex;
      justify-content: flex-end;
      align-items: center;
      gap: 12px;
      margin-top: 28px;
      border-top: 1px solid var(--border);
      padding-top: 20px;
    }

    .btn {
      padding: 10px 18px;
      border-radius: var(--r-sm);
      font-family: var(--font-sans);
      font-size: 13px;
      font-weight: 600;
      cursor: pointer;
      transition: all 0.15s;
      display: inline-flex;
      align-items: center;
      gap: 8px;
      text-decoration: none;
    }

    .btn-cancel {
      border: 1px solid var(--border);
      background: var(--surface);
      color: var(--text-2);
    }

    .btn-cancel:hover {
      background: var(--surface-raised);
      color: var(--text);
    }

    .btn-submit {
      border: 1px solid var(--accent);
      background: var(--accent);
      color: #fff;
    }

    .btn-submit:hover {
      background: var(--accent-hover);
      border-color: var(--accent-hover);
    }

    .btn-submit:disabled {
      opacity: 0.6;
      cursor: not-allowed;
    }

    /* ── Success Modal ── */
    .modal-backdrop {
      display: none;
      position: fixed;
      inset: 0;
      z-index: 10000;
      background: rgba(20,20,30,0.4);
      backdrop-filter: blur(6px);
      align-items: center;
      justify-content: center;
      padding: 16px;
    }
    .modal-backdrop.open {
      display: flex;
    }
    .modal {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-lg);
      padding: 32px;
      width: 100%;
      max-width: 460px;
      box-shadow: 0 16px 48px rgba(20,16,8,0.18);
      animation: modal-in 0.2s ease-out;
      text-align: center;
    }
    .modal-icon {
      font-size: 40px;
      margin-bottom: 16px;
    }
    .modal-title {
      font-size: 18px;
      font-weight: 700;
      color: var(--text);
      margin-bottom: 8px;
      letter-spacing: -0.01em;
    }
    .modal-text {
      font-size: 13px;
      color: var(--text-dim);
      margin-bottom: 24px;
      line-height: 1.6;
    }
    .complaint-badge {
      display: inline-block;
      font-family: var(--font-mono);
      font-weight: 700;
      font-size: 15px;
      background: var(--accent-soft);
      border: 1px dashed var(--accent);
      color: var(--accent);
      padding: 6px 14px;
      border-radius: var(--r-xs);
      margin: 4px 0 12px 0;
    }
    .modal-actions {
      display: flex;
      justify-content: center;
      gap: 12px;
    }

    /* Spinner */
    .spinner {
      display: inline-block;
      width: 14px;
      height: 14px;
      border: 2px solid rgba(255,255,255,0.3);
      border-top-color: #fff;
      border-radius: 50%;
      animation: spin 0.8s linear infinite;
    }
    @keyframes spin {
      to { transform: rotate(360deg); }
    }

    .banner {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 10px 14px;
      border-radius: var(--r-sm);
      font-size: 13px;
      margin-bottom: 20px;
      border: 1px solid transparent;
      animation: banner-in 0.15s ease-out;
    }
    @keyframes banner-in {
      from { opacity: 0; transform: translateY(-4px); }
      to { opacity: 1; transform: translateY(0); }
    }
    .banner.error {
      background: var(--danger-dim);
      border-color: rgba(220,38,38,0.15);
      color: var(--danger);
    }
  </style>
</head>
<body>

  <div class="form-container">
    <div class="header">
      <h1 class="title">📝 Manual Complaint Registration</h1>
      <p class="subtitle">Create a local complaint. Only Name and Mobile No. are compulsory.</p>
    </div>

    <div id="alertBanner" class="banner error" style="display:none"></div>

    <form id="registerForm">
      <div class="form-row">
        <div class="form-group">
          <label class="label" for="complainantName">Complainant Name <span class="required-star">*</span></label>
          <input type="text" id="complainantName" class="input-field" placeholder="Enter complainant name" required autocomplete="off">
        </div>
        <div class="form-group">
          <label class="label" for="mobileNo">Mobile Number <span class="required-star">*</span></label>
          <input type="tel" id="mobileNo" class="input-field" placeholder="e.g. 9876543210" required autocomplete="off">
        </div>
      </div>

      <div class="form-row">
        <div class="form-group">
          <label class="label" for="consumerNo">Consumer Number</label>
          <input type="text" id="consumerNo" class="input-field" placeholder="Enter consumer account number" autocomplete="off">
        </div>
        <div class="form-group">
          <label class="label" for="belt">Belt Assignment</label>
          <select id="belt" class="input-field">
            <option value="auto" selected>Auto Assign (Default)</option>
          </select>
        </div>
      </div>

      <div class="form-row">
        <div class="form-group">
          <label class="label" for="exactLocation">Exact Location / Address</label>
          <input type="text" id="exactLocation" class="input-field" placeholder="Enter house no, landmark, street" autocomplete="off">
        </div>
        <div class="form-group">
          <label class="label" for="area">Area / Locality</label>
          <input type="text" id="area" class="input-field" placeholder="Enter village or sub-area" autocomplete="off">
        </div>
      </div>

      <div class="form-group full-width">
        <label class="label" for="description">Complaint Details / Description</label>
        <textarea id="description" class="input-field" placeholder="Describe the power failure or complaint details..." rows="3"></textarea>
      </div>

      <div class="actions">
        <a href="/" class="btn btn-cancel">← Back to Dashboard</a>
        <button type="submit" id="submitBtn" class="btn btn-submit">
          <span class="spinner" id="submitSpinner" style="display:none"></span>
          Register Complaint
        </button>
      </div>
    </form>
  </div>

  <!-- Success Modal -->
  <div id="successModal" class="modal-backdrop" role="dialog" aria-modal="true">
    <div class="modal">
      <div class="modal-icon">✅</div>
      <div class="modal-title">Complaint Registered</div>
      <div class="modal-text">
        The complaint has been successfully registered and alerts have been sent to Telegram and WhatsApp.
        <br>
        <div class="complaint-badge" id="modalComplaintID">VLDXXXXXXXX</div>
      </div>
      <div class="modal-actions">
        <button id="modalNewBtn" class="btn btn-submit" type="button">Register Another</button>
        <a href="/" class="btn btn-cancel">Go to Dashboard</a>
      </div>
    </div>
  </div>

  <script>
    const BELTS = {{.BeltsJSON}};
    
    const $ = (id) => document.getElementById(id);
    const form = $("registerForm");
    const beltSelect = $("belt");
    const submitBtn = $("submitBtn");
    const submitSpinner = $("submitSpinner");
    const alertBanner = $("alertBanner");
    const successModal = $("successModal");
    const modalComplaintID = $("modalComplaintID");
    const modalNewBtn = $("modalNewBtn");

    // Populate Belt dropdown
    BELTS.forEach(beltName => {
      const opt = document.createElement("option");
      opt.value = beltName;
      opt.textContent = beltName;
      beltSelect.appendChild(opt);
    });

    const setBanner = (msg) => {
      if (msg) {
        alertBanner.textContent = msg;
        alertBanner.style.display = "flex";
      } else {
        alertBanner.style.display = "none";
      }
    };

    form.addEventListener("submit", (e) => {
      e.preventDefault();
      setBanner(null);

      const complainantName = $("complainantName").value.trim();
      const mobileNo = $("mobileNo").value.trim();
      const consumerNo = $("consumerNo").value.trim();
      const beltVal = $("belt").value;
      const exactLocation = $("exactLocation").value.trim();
      const area = $("area").value.trim();
      const description = $("description").value.trim();

      if (!complainantName || !mobileNo) {
        setBanner("Name and Mobile number are compulsory.");
        return;
      }

      submitBtn.disabled = true;
      submitSpinner.style.display = "inline-block";

      fetch("/register-local", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          complainant_name: complainantName,
          mobile_no: mobileNo,
          consumer_no: consumerNo,
          exact_location: exactLocation,
          area: area,
          description: description,
          belt: beltVal
        })
      })
      .then(async (resp) => {
        const data = await resp.json().catch(() => ({}));
        if (!resp.ok) {
          throw new Error(data.error || "Status " + resp.status);
        }
        
        // Show success modal
        modalComplaintID.textContent = data.complaint_id || "VLD-SUCCESS";
        successModal.classList.add("open");
      })
      .catch((err) => {
        setBanner("Registration failed: " + err.message);
      })
      .finally(() => {
        submitBtn.disabled = false;
        submitSpinner.style.display = "none";
      });
    });

    modalNewBtn.addEventListener("click", () => {
      form.reset();
      beltSelect.value = "auto";
      successModal.classList.remove("open");
      $("complainantName").focus();
    });
  </script>
</body>
</html>
`))
