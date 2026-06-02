async function simulate(code) {
    const responseBox = document.getElementById('response-box');
    const educationPanel = document.getElementById('education-panel');

    // Reset state
    responseBox.className = 'response-box';
    educationPanel.className = 'education-panel';
    responseBox.innerHTML = '<p style="color:#444">&gt; Sending request...</p>';

    try {
        const response = await fetch(`/simulate/${code}`);
        const data = await response.json();

        // Display the raw response
        responseBox.innerHTML = `
            <span style="color:#888">&gt; HTTP Response Received</span>
            <br><br>
            <span style="color:#fff">Status:</span> ${data.status}
            <br>
            <span style="color:#fff">Error:</span>  ${data.error}
            <br>
            <span style="color:#fff">Message:</span> ${data.message}
            <br><br>
            <span style="color:#888">// ${data.tip}</span>
        `;

        // Style based on code
        if (code === 404) {
            responseBox.classList.add('error-404');
            educationPanel.className = 'education-panel panel-404 visible';
            educationPanel.innerHTML = `
                <h3>&gt; UNDERSTANDING 404 NOT FOUND_</h3>
                <p>A <strong style="color:#ff4141">404</strong> means the server was reached successfully, 
                but the specific resource (page, file, or endpoint) does not exist at that URL.</p>
                <p style="margin-top:0.8rem">Think of it like going to a library and asking for a book 
                that was never written. The library exists. The librarian is there. But the book? Gone.</p>
                <p class="fix">&gt; Common Fixes: Check the URL for typos. The page may have moved (check for 301 redirects). The resource may have been deleted.</p>
            `;
        }

        if (code === 403) {
            responseBox.classList.add('error-403');
            educationPanel.className = 'education-panel panel-403 visible';
            educationPanel.innerHTML = `
                <h3>&gt; UNDERSTANDING 403 FORBIDDEN_</h3>
                <p>A <strong style="color:#ffd700">403</strong> means the server was reached and the 
                resource EXISTS, but you do not have permission to access it.</p>
                <p style="margin-top:0.8rem">Think of it like going to a library and asking for a book 
                that is locked in the restricted section. The book is there. But you are not on the list.</p>
                <p class="fix">&gt; Common Fixes: Check your authentication token. You may need to log in. The server may be blocking your IP address or User-Agent.</p>
            `;
        }

    } catch (err) {
        responseBox.innerHTML = `<span style="color:#ff4141">&gt; Request failed: ${err.message}</span>`;
    }
}
