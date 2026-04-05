import { useSettings } from "../../app/settings";

type Props = {
  isOpen: boolean;
  onClose: () => void;
};

export function SettingsModal({ isOpen, onClose }: Props) {
  const { theme, setTheme, quoteCurrency, setQuoteCurrency } = useSettings();

  if (!isOpen) return null;

  return (
    <>
      <div className="modal-backdrop" onClick={onClose} />
      <div className="modal">
        <div className="modal-header">
          <h3>Settings</h3>
          <button className="icon-btn" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        <div className="modal-body">
          <div className="setting-group">
            <label>Theme</label>
            <div className="toggle-group">
              <button 
                className={theme === "dark" ? "active" : ""} 
                onClick={() => setTheme("dark")}
              >
                Dark
              </button>
              <button 
                className={theme === "light" ? "active" : ""} 
                onClick={() => setTheme("light")}
              >
                Light
              </button>
            </div>
          </div>
          
          <div className="setting-group">
            <label>Quote Currency</label>
            <div className="toggle-group">
              <button 
                className={quoteCurrency === "EUR" ? "active" : ""} 
                onClick={() => setQuoteCurrency("EUR")}
              >
                EUR
              </button>
              <button 
                className={quoteCurrency === "USD" ? "active" : ""} 
                onClick={() => setQuoteCurrency("USD")}
              >
                USD
              </button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
