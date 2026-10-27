package cn.linji.clinical;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.webkit.MimeTypeMap;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;

/** Exposes authenticated downloads through temporary read grants, without passing a server token. */
public final class CacheProvider extends ContentProvider {
    @Override public boolean onCreate(){return true;}
    private File file(Uri uri) throws FileNotFoundException {
        try {
            File dir=new File(providerContext().getCacheDir(),"exports").getCanonicalFile();String name=uri.getLastPathSegment();
            if(name==null)throw new FileNotFoundException("Missing file");File f=new File(dir,name).getCanonicalFile();
            if(!dir.equals(f.getParentFile()))throw new FileNotFoundException("Invalid path");return f;
        }catch(IOException e){throw new FileNotFoundException("Invalid file");}
    }
    private android.content.Context providerContext(){android.content.Context c=getContext();if(c==null)throw new IllegalStateException();return c;}
    @Override public ParcelFileDescriptor openFile(Uri uri,String mode)throws FileNotFoundException{
        if(!"r".equals(mode))throw new FileNotFoundException("Read only");return ParcelFileDescriptor.open(file(uri),ParcelFileDescriptor.MODE_READ_ONLY);
    }
    @Override public String getType(Uri uri){String name=uri.getLastPathSegment();String mime=MimeTypeMap.getSingleton().getMimeTypeFromExtension(name==null?"":name.substring(name.lastIndexOf('.')+1).toLowerCase(java.util.Locale.ROOT));return mime==null?"application/octet-stream":mime;}
    @Override public Cursor query(Uri uri,String[] projection,String selection,String[] args,String sort){try{File f=file(uri);MatrixCursor c=new MatrixCursor(new String[]{OpenableColumns.DISPLAY_NAME,OpenableColumns.SIZE});c.addRow(new Object[]{f.getName(),f.length()});return c;}catch(FileNotFoundException e){return null;}}
    @Override public Uri insert(Uri uri,ContentValues values){throw new UnsupportedOperationException();}
    @Override public int update(Uri uri,ContentValues values,String where,String[] args){throw new UnsupportedOperationException();}
    @Override public int delete(Uri uri,String where,String[] args){throw new UnsupportedOperationException();}
}
