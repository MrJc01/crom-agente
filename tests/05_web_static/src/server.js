const express = require('express');
const bodyParser = require('body-parser');

const app = express();
const PORT = 3000;

// Middleware
app.use(bodyParser.json());

let notes = [];
let nextId = 1;

// GET /api/notes - Lista todas as notas
app.get('/api/notes', (req, res) => {
    res.json(notes);
});

// POST /api/notes - Cria uma nova nota
app.post('/api/notes', (req, res) => {
    const { title, content } = req.body;
    if (!title || !content) {
        return res.status(400).json({ error: 'Title and content are required' });
    }
    const newNote = {
        id: nextId++,
        title,
        content,
        createdAt: new Date().toISOString()
    };
    notes.push(newNote);
    res.status(201).json(newNote);
});

// DELETE /api/notes/:id - Remove uma nota
app.delete('/api/notes/:id', (req, res) => {
    const { id } = req.params;
    const initialLength = notes.length;
    notes = notes.filter(note => note.id !== parseInt(id));

    if (notes.length === initialLength) {
        return res.status(404).json({ error: 'Note not found' });
    }
    res.status(204).send(); // No Content
});

app.listen(PORT, () => {
    console.log(`Server running on http://localhost:${PORT}`);
});
