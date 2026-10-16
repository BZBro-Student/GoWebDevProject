# Translation Game

**Course:** CS 408 - Full Stack Web Development (Fall 2026)  
**Authors:** Broden Z. Benson, Shane Panter  

## Overview
The Translation Game is a full-stack web application designed to foster curiosity, improve pattern recognition, and develop linguistic reasoning. Users parse procedurally generated documents written in a fictional language and construct an evolving English dictionary to translate the text. Inspired by the indie puzzle game *Chants of Sennaar*, this project creates a framework for an expandable foreign dictionary and diegetic puzzle solving.

## Target Audience
* **Primary Audience:** Individuals interested in linguistics, code-breaking, and puzzle games.
* **Linguists:** Users seeking a simple semantic challenge.
* **Code Breakers:** Individuals interested in decoding natural language patterns.
* **Gamers:** Players looking for an engaging, low-stakes deductive puzzle.

## Tech Stack
* **Backend:** Go (`net/http` standard library)
* **Database:** SQLite relational database
* **Templating & Interactivity:** Templ HTML templating and HTMX
* **Styling:** Tailwind CSS
* **Authentication:** GitHub OAuth
* **Deployment & CI/CD:** AWS EC2 (Ubuntu), Bash setup scripts, GitHub Actions

## Prerequisites
Before running the application, ensure the following tools are installed on your environment:
* **Go:** Version 1.22+ installed and available in system PATH.
* **Tailwind CSS v4 Standalone CLI:** Binary downloaded and installed in system PATH (required for compilation by start script).

## Application Features & Navigation

### Global Features
* GitHub OAuth integration for user authentication.
* Session tracking for user state management.
* Navigation header and a hideable right-hand control panel with tab-specific controls.

### User Tabs
* **Desk Tab:** Landing page and main reading workspace. Displays documents alongside a live working translation updated automatically via the user's saved dictionary. The side panel provides document search and live chat.
* **Dictionary Tab:** Allows users to view, search, and manage custom translation definitions. Side panel supports filtering by untranslated words or English meanings.
* **Tutorial Tab:** Static guide introducing basic gameplay mechanics and starter vocabulary.
* **Community Tab:** Discussion forum split into general discussion and a translation practice area restricted to verified database vocabulary.
* **Help Tab:** Static page providing additional project and gameplay information.
* **Settings Tab:** Form interface for personal preferences, dark/light mode toggles, progress resets, and account deletion.

### Admin Features
* **Admin Tab:** Content upload interface where uploaded text files are automatically parsed, translated, and stored by the server. Supports file deletion with database cleanups that preserve user library progress.

## Database Schema

### User Table
* `Id` (TEXT): Unique user identifier.
* `Username` (TEXT): Visual display name.
* `Is_admin` (INTEGER): Admin permissions flag (0 = default user, 1 = admin).
* `Creation_time` (TEXT): Account creation timestamp.

### User Dictionary Table
* `Id` (TEXT): User reference ID.
* `User_definition` (TEXT): User's custom translation definition.
* `Foreign_word` (TEXT): Fictional target word being translated.
* `Update_time` (TEXT): Timestamp of definition entry.

### Server Dictionary Table
* `Foreign_word` (TEXT): Fictional language vocabulary word.
* `English_translations` (TEXT): Accepted English semantic equivalents.

### Document Table
* `Doc_id` (TEXT): Unique document identifier.
* `Title` (TEXT): Document title.
* `Content` (TEXT): Document body text.
* `Timestamp` (TEXT): Upload/creation timestamp.

### Global Chat Message Table
* `Message_id` (TEXT): Unique message identifier.
* `Sender_id` (TEXT): User ID of the sender.
* `Content` (TEXT): Message text payload.
* `Timestamp` (TEXT): Timestamp of message submission.

### Forum Thread Table
* `Thread_id` (TEXT): Primary key for discussion topics.
* `Category_id` (TEXT): Forum section identifier.
* `Author_id` (TEXT): User ID of post author.
* `Title` (TEXT): Thread title.
* `Locked` (INTEGER): Lock status for replies.
* `Created_at` (TEXT): Thread creation timestamp.

### Forum Replies Table
* `Thread_id` (TEXT): Parent thread identifier.
* `Reply_id` (TEXT): Unique reply identifier.
* `Title` (TEXT): Reply header.
* `Author_id` (TEXT): User ID of reply author.
* `Text` (TEXT): Content of reply.
* `Created_at` (TEXT): Timestamp of reply.

## Deployment & Setup

### AWS EC2 Setup
1. Launch an AWS EC2 instance running Ubuntu.
2. Connect to the instance using SSH.
3. Execute the setup Bash script to install Go, SQLite, standalone Tailwind CSS, and project dependencies.
4. Execute the configuration script to initialize environment variables, create databases, and launch the web server.
5. Access the application on the configured network port.

## Project Milestones

* **Checkpoint 1:** Directory structure setup, GitHub Actions deployment setup, static HTML templates, and route handlers.
* **Checkpoint 2:** EC2 deployment via Bash scripts, SQLite integration, GitHub OAuth integration, and session tracking.
* **Checkpoint 3:** Server translation logic, Admin tab file upload implementation, and database test population.
* **Checkpoint 4:** Desk tab document rendering, Dictionary tab functionality, and automated translation replacement engine.
* **Checkpoint 5:** Community forum features, basic HTMX dynamic chat, and dynamic side panel routing.
* **Checkpoint 6:** Tutorial, Help, and Settings pages with UI styling (light/dark mode toggle).
* **Checkpoint 7:** Multi-client testing and bug resolution.
* **Stretch Goal:** WebSocket-based live global chat.